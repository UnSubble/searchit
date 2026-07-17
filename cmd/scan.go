package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/console"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/resolver"
	scanProfile "github.com/unsubble/searchit/internal/profile/scan"
	"github.com/unsubble/searchit/internal/progress"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/targets"
	"github.com/unsubble/searchit/internal/wordlist"
	"golang.org/x/time/rate"
)

var (
	flagURL             string
	flagURLFile         string
	flagWordlist        string
	flagThreads         int
	flagTimeout         int
	flagRecursive       bool
	flagMaxDepth        uint16
	flagStrategy        string
	flagExcludeStatus   string
	flagRecurseOn       string
	flagNormalizePaths  bool
	flagCollapseSlashes bool
	// flagOutput is the output file path ("" means stdout).
	flagOutput string
	// flagFormat is the explicit output format name.
	flagFormat          string
	flagQuiet           bool
	flagIncludeSize     string
	flagExcludeSize     string
	flagIncludeHeaders  []string
	flagExcludeHeaders  []string
	flagDelay           string
	flagRate            float64
	flagConnectTimeout  string
	flagProfiles        []string
	flagNoProgress      bool
	flagTech            string
	flagFollowRedirects bool
	flagAdaptive        bool

	flagScanMethod  string
	flagScanData    string
	flagScanHeaders []string
	flagScanCookie  string
	flagScanProxy   string

	flagMatchStatus   string
	flagFilterStatus  string
	flagMatchSize     string
	flagFilterSize    string
	flagMatchRegex    []string
	flagFilterRegex   []string
	flagMatchContent  []string
	flagFilterContent []string

	resolvedTargets []string

	testHookConfigApplied func(config.Config)
)

// techProfiles is the built-in registry of supported technology identifiers.
// Keys are canonical lowercase IDs; values are human-readable display names.
// Add new technologies here — no other file needs to change.
var techProfiles = map[string]string{
	"angular":   "Angular",
	"aspnet":    "ASP.NET",
	"django":    "Django",
	"express":   "Express",
	"flask":     "Flask",
	"go":        "Go",
	"laravel":   "Laravel",
	"nextjs":    "Next.js",
	"nuxt":      "Nuxt",
	"react":     "React",
	"spring":    "Spring Boot",
	"vue":       "Vue",
	"wordpress": "WordPress",
}

// lookupTech returns the config.TechProfile for the given ID (case-insensitive).
func lookupTech(id string) (config.TechProfile, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	name, ok := techProfiles[key]
	if !ok {
		return config.TechProfile{}, false
	}
	return config.TechProfile{ID: key, DisplayName: name}, true
}

// supportedTechIDs returns a sorted, comma-separated list of supported tech IDs.
func supportedTechIDs() string {
	ids := make([]string, 0, len(techProfiles))
	for id := range techProfiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return strings.Join(ids, ", ")
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan a target URL",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if flagThreads < 1 {
			return fmt.Errorf("threads must be at least 1")
		}
		if flagMaxDepth < 1 {
			return fmt.Errorf("max depth must be at least 1")
		}
		if flagStrategy != "bfs" && flagStrategy != "dfs" {
			return fmt.Errorf("invalid strategy %q: must be bfs or dfs", flagStrategy)
		}
		if cmd.Flags().Changed("max-depth") && !flagRecursive {
			return fmt.Errorf("max-depth requires recursive scanning to be enabled")
		}
		if cmd.Flags().Changed("strategy") && !flagRecursive {
			return fmt.Errorf("strategy requires recursive scanning to be enabled")
		}
		if cmd.Flags().Changed("recurse-on") && !flagRecursive {
			return fmt.Errorf("recurse-on requires recursive scanning to be enabled")
		}

		if flagRecursive {
			if _, err := status.Parse(flagRecurseOn); err != nil {
				return fmt.Errorf("invalid recurse-on: %w", err)
			}
		}

		// Validate --format if explicitly provided.
		if cmd.Flags().Changed("format") {
			if _, err := output.Parse(flagFormat); err != nil {
				return fmt.Errorf("invalid --format: %w", err)
			}
		}

		// --output is a file path; validate that it is not an existing directory.
		if flagOutput != "" {
			if fi, err := os.Stat(flagOutput); err == nil && fi.IsDir() {
				return fmt.Errorf("--output %q is a directory; provide a file path", flagOutput)
			}
		}

		if _, err := size.Parse(flagIncludeSize); err != nil {
			return fmt.Errorf("invalid include-size: %w", err)
		}
		if _, err := size.Parse(flagExcludeSize); err != nil {
			return fmt.Errorf("invalid exclude-size: %w", err)
		}

		if flagDelay != "" {
			if _, err := time.ParseDuration(flagDelay); err != nil {
				return fmt.Errorf("invalid delay: %w", err)
			}
		}

		if cmd.Flags().Changed("rate") && flagRate <= 0 {
			return fmt.Errorf("rate must be greater than 0")
		}

		if flagConnectTimeout != "" {
			ct, err := time.ParseDuration(flagConnectTimeout)
			if err != nil {
				return fmt.Errorf("invalid connect-timeout: %w", err)
			}
			if ct < 0 {
				return fmt.Errorf("connect-timeout cannot be negative")
			}
		}

		for _, h := range flagIncludeHeaders {
			if err := validateHeaderFlag(h); err != nil {
				return fmt.Errorf("invalid include-header: %w", err)
			}
		}
		for _, h := range flagExcludeHeaders {
			if err := validateHeaderFlag(h); err != nil {
				return fmt.Errorf("invalid exclude-header: %w", err)
			}
		}

		for _, h := range flagScanHeaders {
			idx := strings.Index(h, "=")
			if idx == -1 {
				idx = strings.Index(h, ":")
			}
			if idx <= 0 || idx == len(h)-1 {
				return fmt.Errorf("invalid header %q: must be in Name: Value or Name=Value format", h)
			}
		}

		if flagScanProxy != "" {
			// Try parsing as url
			if _, err := url.Parse(flagScanProxy); err != nil {
				return fmt.Errorf("invalid proxy URL %q: %w", flagScanProxy, err)
			}
		}

		if flagMatchStatus != "" {
			if _, err := status.Parse(flagMatchStatus); err != nil {
				return fmt.Errorf("invalid match-status (--mc): %w", err)
			}
		}
		if flagFilterStatus != "" {
			if _, err := status.Parse(flagFilterStatus); err != nil {
				return fmt.Errorf("invalid filter-status (--fc): %w", err)
			}
		}
		if flagMatchSize != "" {
			if _, err := size.Parse(flagMatchSize); err != nil {
				return fmt.Errorf("invalid match-size (--ms): %w", err)
			}
		}
		if flagFilterSize != "" {
			if _, err := size.Parse(flagFilterSize); err != nil {
				return fmt.Errorf("invalid filter-size (--fs): %w", err)
			}
		}
		for _, rx := range flagMatchRegex {
			if _, err := regexp.Compile(rx); err != nil {
				return fmt.Errorf("invalid match-regex (--mr) %q: %w", rx, err)
			}
		}
		for _, rx := range flagFilterRegex {
			if _, err := regexp.Compile(rx); err != nil {
				return fmt.Errorf("invalid filter-regex (--fr) %q: %w", rx, err)
			}
		}

		resolvedTargets = nil
		if flagURL != "" {
			for _, val := range strings.Split(flagURL, ",") {
				t := strings.TrimSpace(val)
				if t != "" {
					resolvedTargets = append(resolvedTargets, t)
				}
			}
		}
		if flagURLFile != "" {
			fileTargets, err := targets.ReadFile(flagURLFile)
			if err != nil {
				return fmt.Errorf("failed to read url file: %w", err)
			}
			resolvedTargets = append(resolvedTargets, fileTargets...)
		}
		if len(resolvedTargets) == 0 {
			return fmt.Errorf("at least one target is required")
		}

		if flagTech != "" {
			if _, ok := lookupTech(flagTech); !ok {
				return fmt.Errorf(
					"unknown technology %q; supported values: %s",
					flagTech, supportedTechIDs(),
				)
			}
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		stats.GlobalInstrumentation.Reset()
		atomic.StoreInt32(&stats.GlobalInstrumentation.Enabled, 1)
		if os.Getenv("SEARCHIT_TRACE") == "1" {
			atomic.StoreInt32(&stats.GlobalInstrumentation.Trace, 1)
		}

		// Ensure SilenceErrors/SilenceUsage are always reset when RunE returns,
		// so that transient profile-load failures do not permanently suppress
		// error output for subsequent invocations within the same test binary.
		defer func() {
			cmd.SilenceErrors = false
			cmd.SilenceUsage = false
		}()

		// 1. Start from defaults.
		cfg := config.Default()

		// 2. Apply profiles (left → right).
		var appliedProfiles []string
		if len(flagProfiles) > 0 {
			store := profile.NewStore()
			res := resolver.New(store)
			resolved, err := res.Resolve(flagProfiles)
			if err != nil {
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
				fmt.Fprintf(cmd.ErrOrStderr(), "failed to load profile:\n%v\n", err)
				return fmt.Errorf("load failed")
			}

			for _, p := range resolved {
				appliedProfiles = append(appliedProfiles, p.Name)

				// Validate (generic)
				if err := profile.Validate(p); err != nil {
					cmd.SilenceErrors = true
					cmd.SilenceUsage = true
					fmt.Fprintf(cmd.ErrOrStderr(), "failed to load profile:\n%v\n", err)
					return fmt.Errorf("validation failed")
				}

				// Validate (tool-specific)
				if v := profile.GetValidator(p.Tool); v != nil {
					if err := v.Validate(p); err != nil {
						cmd.SilenceErrors = true
						cmd.SilenceUsage = true
						fmt.Fprintf(cmd.ErrOrStderr(), "failed to load profile:\n%v\n", err)
						return fmt.Errorf("validation failed")
					}
				}

				// Decode into scan.Overlay
				var overlay scanProfile.Overlay
				if err := p.Decode(&overlay); err != nil {
					cmd.SilenceErrors = true
					cmd.SilenceUsage = true
					fmt.Fprintf(cmd.ErrOrStderr(), "failed to load profile:\n%v\n", err)
					return fmt.Errorf("decode failed")
				}

				// Apply
				scanProfile.Apply(&cfg, overlay)
			}
		}

		// 3. Apply CLI flags (highest priority — only if explicitly changed).
		applyCLIOverrides(cmd, &cfg)

		if cfg.OutputFormat == "text" && !cfg.Quiet && len(appliedProfiles) > 0 {
			fmt.Println("[*] Profiles:")
			for _, name := range appliedProfiles {
				fmt.Printf("    %s\n", name)
			}
		}
		if cfg.OutputFormat == "text" && !cfg.Quiet && cfg.TechProfile != nil {
			fmt.Printf("[*] Tech profile: %s\n", cfg.TechProfile.ID)
		}

		if testHookConfigApplied != nil {
			testHookConfigApplied(cfg)
			return nil
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		appState := app.New(ctx, cfg)

		var reader wordlist.Reader
		if cfg.Wordlist == "" {
			reader = wordlist.EmbeddedReader{}
		} else {
			reader = wordlist.FileReader{Path: cfg.Wordlist}
		}

		// Count total words if the reader supports it (for progress bar ETA estimation).
		var totalWords int
		if countable, ok := reader.(wordlist.Countable); ok {
			if count, err := countable.Count(); err == nil {
				totalWords = count
			}
		}

		// Determine the output writer (file or stdout).
		outWriter := os.Stdout
		if cfg.OutputFile != "" {
			f, err := os.OpenFile(cfg.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return fmt.Errorf("cannot open output file: %w", err)
			}
			defer f.Close()
			outWriter = f
		}

		// Construct the formatter from the resolved format name.
		fmt_, err := output.Parse(cfg.OutputFormat)
		if err != nil {
			// Fallback — should not happen after validation, but be safe.
			fmt_ = output.FormatText
		}
		fmttr := output.New(fmt_, outWriter, cfg.Quiet)
		defer fmttr.Close()

		var limiter *rate.Limiter
		if cfg.Rate > 0 {
			limiter = rate.NewLimiter(rate.Limit(cfg.Rate), 1)
		}

		customHeaders, err := parseFuzzHeaderFlags(cfg.Headers)
		if err != nil {
			return err
		}
		parsedCookies := parseCookies(cfg.Cookies)

		fs, err := filter.NewFilterSuite(
			cfg.Status.Include.String(),
			cfg.Status.Exclude.String(),
			cfg.IncludeSize.String(),
			cfg.ExcludeSize.String(),
			cfg.MatchRegex,
			cfg.FilterRegex,
			cfg.MatchContent,
			cfg.FilterContent,
		)
		if err != nil {
			return err
		}

		for _, targetURL := range cfg.URLs {
			if cfg.OutputFormat == "text" {
				fmt.Printf("[*] Target: %s\n", targetURL)
			}

			scanCtx, scanCancel := context.WithCancel(ctx)
			collector := stats.NewCollector()
			if totalWords > 0 {
				collector.SetQueuedJobs(int64(totalWords))
			}
			var progMgr *progress.Manager
			var renderer *progress.ANSIRenderer
			var progCmdChan chan console.Command

			enableProgress := shouldEnableProgress(cfg, flagNoProgress)
			// Interactive keyboard controls require stdin to also be a terminal.
			interactive := enableProgress && console.IsTerminal(os.Stdin.Fd())

			var progDone chan struct{}
			if enableProgress {
				modeStr := "Single target"
				if cfg.Recursive {
					modeStr = fmt.Sprintf("Recursive (%s)", strings.ToUpper(cfg.Strategy.String()))
				}
				renderer = progress.NewANSIRenderer(os.Stdout, targetURL, appliedProfiles, modeStr)
				progMgr = progress.NewManager(collector, renderer, 1*time.Second)
				progMgr.ConfiguredThreads = cfg.Threads
				progMgr.Formatter = fmttr

				if interactive {
					consoleCtrl := console.NewController(os.Stdin)
					progCmdChan = make(chan console.Command, 10)
					go consoleCtrl.Start(scanCtx)

					go func() {
						for cmd := range consoleCtrl.Commands() {
							switch cmd {
							case console.CommandProgress, console.CommandStats:
								select {
								case progCmdChan <- cmd:
								default:
								}
							case console.CommandStop:
								scanCancel()
							}
						}
						close(progCmdChan)
					}()
				}

				progDone = make(chan struct{})
				go func() {
					progMgr.Start(scanCtx, progCmdChan)
					close(progDone)
				}()
			}

			if cfg.Recursive {
				if cfg.OutputFormat == "text" {
					fmt.Println("[*] Recursive scan enabled")
					fmt.Printf("[*] Strategy: %s\n", cfg.Strategy.String())
					fmt.Printf("[*] Max depth: %d\n", cfg.MaxDepth)
					fmt.Printf("[*] Recurse on: %s\n", formatRecurseOn(cfg))
					if !cfg.RecurseOn.Match(404) {
						fmt.Println("[*] 404 responses are ignored by default.")
					}
				}

				var fpCache *fingerprint.Cache
				if cfg.Adaptive {
					fpCache = appState.FingerprintCache
				}

				seeds := []string{targetURL}
				manager := recursion.NewManager(
					appState.HTTPClient,
					appState.Config.Status.Exclude,
					reader,
					cfg.Strategy,
					cfg.MaxDepth,
					cfg.RecurseOn,
					cfg.Paths.NormalizePaths,
					cfg.Paths.CollapseSlashes,
					cfg.IncludeSize,
					cfg.ExcludeSize,
					mapHeaders(cfg.IncludeHeaders),
					mapHeaders(cfg.ExcludeHeaders),
					cfg.Delay,
					limiter,
					fpCache,
				)
				manager.SetRequestManipulation(cfg.Method, []byte(cfg.Data), customHeaders, parsedCookies)
				manager.SetFilterSuite(fs)
				manager.SetStats(collector)
				results := manager.Run(scanCtx, seeds, cfg.Threads)
				for r := range results {
					if r.Accepted {
						if progMgr != nil {
							progMgr.HandleResult(r)
						} else {
							_ = fmttr.Print(r)
						}
					}
				}
			} else {
				jobs := make(chan engine.Job, cfg.Threads)
				results := engine.Start(
					scanCtx,
					appState.HTTPClient,
					fs,
					mapHeaders(cfg.IncludeHeaders),
					mapHeaders(cfg.ExcludeHeaders),
					cfg.Threads,
					cfg.Delay,
					limiter,
					cfg.Method,
					[]byte(cfg.Data),
					customHeaders,
					parsedCookies,
					jobs,
					collector,
				)

				go func() {
					p := wordlist.Producer{
						BaseURL:         targetURL,
						Reader:          reader,
						NormalizePaths:  cfg.Paths.NormalizePaths,
						CollapseSlashes: cfg.Paths.CollapseSlashes,
					}
					_ = p.Produce(scanCtx, jobs)
				}()

				for r := range results {
					atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
					if r.Accepted {
						atomic.AddInt64(&stats.GlobalInstrumentation.ResultsAccepted, 1)
						if progMgr != nil {
							progMgr.HandleResult(r)
						} else {
							_ = fmttr.Print(r)
						}
					} else {
						atomic.AddInt64(&stats.GlobalInstrumentation.ResultsRejected, 1)
					}
					collector.DecrementQueuedJobs()
				}
			}
			if !cfg.Quiet {
				stats.GlobalInstrumentation.PrintReconciliation()
			}
			scanCancel()
			if progDone != nil {
				<-progDone
			}
			if renderer != nil {
				_ = renderer.Close()
			}
		}

		return nil
	},
}

// applyCLIOverrides applies CLI flag values to cfg, but only for flags
// that the user explicitly provided. This ensures that profile values
// are not overridden by default flag values.
func applyCLIOverrides(cmd *cobra.Command, cfg *config.Config) {
	cfg.URLs = resolvedTargets

	if cmd.Flags().Changed("wordlist") {
		cfg.Wordlist = flagWordlist
	}
	if cmd.Flags().Changed("threads") {
		cfg.Threads = flagThreads
	}
	if cmd.Flags().Changed("timeout") {
		cfg.Timeout = time.Duration(flagTimeout) * time.Second
	}
	if cmd.Flags().Changed("delay") {
		if d, err := time.ParseDuration(flagDelay); err == nil {
			cfg.Delay = d
		}
	}
	if cmd.Flags().Changed("rate") {
		cfg.Rate = flagRate
	}
	if cmd.Flags().Changed("connect-timeout") {
		if d, err := time.ParseDuration(flagConnectTimeout); err == nil {
			cfg.ConnectTimeout = d
		}
	}
	if cmd.Flags().Changed("recursive") {
		cfg.Recursive = flagRecursive
	}
	if cmd.Flags().Changed("max-depth") {
		cfg.MaxDepth = flagMaxDepth
	}
	if cmd.Flags().Changed("strategy") {
		if s, err := recursion.ParseStrategy(flagStrategy); err == nil {
			cfg.Strategy = s
		}
	}
	if cmd.Flags().Changed("mc") {
		if f, err := status.Parse(flagMatchStatus); err == nil {
			cfg.Status.Include = f
		}
	}
	if cmd.Flags().Changed("fc") {
		if f, err := status.Parse(flagFilterStatus); err == nil {
			cfg.Status.Exclude = f
		}
	} else if cmd.Flags().Changed("exclude-status") {
		if f, err := status.Parse(flagExcludeStatus); err == nil {
			cfg.Status.Exclude = f
		}
	}
	if cmd.Flags().Changed("recurse-on") {
		if f, err := status.Parse(flagRecurseOn); err == nil {
			cfg.RecurseOn = f
		}
	}
	if cmd.Flags().Changed("normalize-paths") {
		cfg.Paths.NormalizePaths = flagNormalizePaths
	}
	if cmd.Flags().Changed("collapse-slashes") {
		cfg.Paths.CollapseSlashes = flagCollapseSlashes
	}
	// --output is the output file path.
	if cmd.Flags().Changed("output") {
		cfg.OutputFile = flagOutput
	}
	// --format is the explicit formatter name.
	// Precedence: explicit --format > auto-detect from extension > default (text).
	if cmd.Flags().Changed("format") {
		if f, err := output.Parse(flagFormat); err == nil {
			cfg.OutputFormat = string(f)
		}
	} else if cfg.OutputFile != "" && !cmd.Flags().Changed("format") {
		// Auto-detect format from file extension when no explicit format is given.
		detected := output.FormatFromPath(cfg.OutputFile)
		cfg.OutputFormat = string(detected)
	}
	if cmd.Flags().Changed("quiet") {
		cfg.Quiet = flagQuiet
	}
	if cmd.Flags().Changed("follow-redirects") {
		cfg.FollowRedirects = flagFollowRedirects
	}
	if cmd.Flags().Changed("ms") {
		if f, err := size.Parse(flagMatchSize); err == nil {
			cfg.IncludeSize = f
		}
	} else if cmd.Flags().Changed("include-size") {
		if f, err := size.Parse(flagIncludeSize); err == nil {
			cfg.IncludeSize = f
		}
	}
	if cmd.Flags().Changed("fs") {
		if f, err := size.Parse(flagFilterSize); err == nil {
			cfg.ExcludeSize = f
		}
	} else if cmd.Flags().Changed("exclude-size") {
		if f, err := size.Parse(flagExcludeSize); err == nil {
			cfg.ExcludeSize = f
		}
	}
	if cmd.Flags().Changed("mr") {
		cfg.MatchRegex = flagMatchRegex
	}
	if cmd.Flags().Changed("fr") {
		cfg.FilterRegex = flagFilterRegex
	}
	if cmd.Flags().Changed("mt") {
		cfg.MatchContent = flagMatchContent
	}
	if cmd.Flags().Changed("ft") {
		cfg.FilterContent = flagFilterContent
	}
	if cmd.Flags().Changed("include-header") {
		cfg.IncludeHeaders = parseHeaderFlags(flagIncludeHeaders)
	}
	if cmd.Flags().Changed("exclude-header") {
		cfg.ExcludeHeaders = parseHeaderFlags(flagExcludeHeaders)
	}
	if cmd.Flags().Changed("method") {
		cfg.Method = flagScanMethod
	}
	if cmd.Flags().Changed("data") {
		cfg.Data = flagScanData
	}
	if cmd.Flags().Changed("header") {
		cfg.Headers = flagScanHeaders
	}
	if cmd.Flags().Changed("cookie") {
		cfg.Cookies = flagScanCookie
	}
	if cmd.Flags().Changed("proxy") {
		cfg.Proxy = flagScanProxy
	}
	if flagTech != "" {
		if p, ok := lookupTech(flagTech); ok {
			cfg.TechProfile = &p
		}
	}
	if cmd.Flags().Changed("adaptive") {
		cfg.Adaptive = flagAdaptive
	}
}

// formatRecurseOn returns a human-readable string for the recurse-on filters.
// Prefers the CLI flag value if set, otherwise falls back to the default.
func formatRecurseOn(_ config.Config) string {
	if flagRecurseOn != "" {
		return flagRecurseOn
	}
	return "200,301,302,403"
}

func init() {
	rootCmd.AddCommand(scanCmd)

	scanCmd.Flags().StringVarP(
		&flagURL,
		"url",
		"u",
		"",
		"target URL",
	)

	scanCmd.Flags().StringVarP(
		&flagWordlist,
		"wordlist",
		"w",
		"",
		"wordlist path",
	)

	scanCmd.Flags().IntVarP(
		&flagThreads,
		"threads",
		"t",
		32,
		"number of concurrent workers",
	)

	scanCmd.Flags().IntVar(
		&flagTimeout,
		"timeout",
		10,
		"timeout in seconds",
	)

	scanCmd.Flags().BoolVarP(
		&flagRecursive,
		"recursive",
		"r",
		false,
		"enable recursive directory scanning",
	)

	scanCmd.Flags().Uint16VarP(
		&flagMaxDepth,
		"max-depth",
		"d",
		3,
		"maximum recursion depth",
	)

	scanCmd.Flags().StringVarP(
		&flagStrategy,
		"strategy",
		"s",
		"bfs",
		"recursion strategy (bfs, dfs)",
	)

	scanCmd.Flags().StringVarP(
		&flagExcludeStatus,
		"exclude-status",
		"x",
		"404",
		"comma-separated status codes to exclude",
	)

	scanCmd.Flags().StringVar(
		&flagRecurseOn,
		"recurse-on",
		"200,301,302,403",
		"comma-separated status codes to recurse on",
	)

	scanCmd.Flags().BoolVar(
		&flagNormalizePaths,
		"normalize-paths",
		false,
		"normalize relative segments in paths (e.g. ././admin -> admin)",
	)

	scanCmd.Flags().BoolVar(
		&flagCollapseSlashes,
		"collapse-slashes",
		false,
		"collapse consecutive slashes (e.g. admin////api -> admin/api)",
	)

	scanCmd.Flags().StringVarP(
		&flagOutput,
		"output",
		"o",
		"",
		"write results to this file (default: stdout); format auto-detected from extension",
	)

	scanCmd.Flags().StringVar(
		&flagFormat,
		"format",
		"text",
		fmt.Sprintf("output format (%s)", strings.Join(output.SupportedFormats(), ", ")),
	)

	scanCmd.Flags().StringVar(
		&flagIncludeSize,
		"include-size",
		"",
		"comma-separated content sizes to include (e.g. 100-200,512)",
	)

	scanCmd.Flags().StringVar(
		&flagExcludeSize,
		"exclude-size",
		"",
		"comma-separated content sizes to exclude (e.g. 0,123)",
	)

	scanCmd.Flags().StringSliceVar(
		&flagIncludeHeaders,
		"include-header",
		nil,
		"HTTP headers to include (e.g. Server=nginx)",
	)

	scanCmd.Flags().StringSliceVar(
		&flagExcludeHeaders,
		"exclude-header",
		nil,
		"HTTP headers to exclude (e.g. Content-Type=text/plain)",
	)

	scanCmd.Flags().BoolVarP(
		&flagQuiet,
		"quiet",
		"q",
		false,
		"print only discovered URLs in text mode",
	)

	scanCmd.Flags().StringVar(
		&flagURLFile,
		"url-file",
		"",
		"load targets from a file (one URL per line)",
	)

	scanCmd.Flags().StringVar(
		&flagDelay,
		"delay",
		"",
		"delay between requests per worker",
	)

	scanCmd.Flags().Float64Var(
		&flagRate,
		"rate",
		0,
		"maximum requests per second across all workers",
	)

	scanCmd.Flags().StringVar(
		&flagConnectTimeout,
		"connect-timeout",
		"3s",
		"timeout for establishing new TCP connections",
	)

	scanCmd.Flags().StringSliceVar(
		&flagProfiles,
		"profile",
		nil,
		"profile(s) to apply (e.g. scan/quick); may be specified multiple times",
	)

	scanCmd.Flags().BoolVar(
		&flagNoProgress,
		"no-progress",
		false,
		"disable the live progress display (progress is enabled automatically when stdout is a terminal)",
	)

	scanCmd.Flags().StringVar(
		&flagTech,
		"tech",
		"",
		"explicitly select a technology profile, bypassing automatic detection (e.g. laravel, spring, wordpress)",
	)

	scanCmd.Flags().BoolVar(
		&flagFollowRedirects,
		"follow-redirects",
		false,
		"follow HTTP redirects",
	)

	scanCmd.Flags().BoolVar(
		&flagAdaptive,
		"adaptive",
		false,
		"enable adaptive scanning (technology detection and path injection)",
	)

	scanCmd.Flags().StringVarP(&flagScanMethod, "method", "X", "GET", "HTTP method to use for requests")
	scanCmd.Flags().StringVar(&flagScanData, "data", "", "POST data body to use for requests")
	scanCmd.Flags().StringSliceVarP(&flagScanHeaders, "header", "H", nil, "HTTP request headers to send (e.g. -H 'Authorization: Bearer X')")
	scanCmd.Flags().StringVarP(&flagScanCookie, "cookie", "b", "", "HTTP request cookies to send (e.g. -b 'session=123')")
	scanCmd.Flags().StringVar(&flagScanProxy, "proxy", "", "HTTP proxy URL to use for requests")

	scanCmd.Flags().StringVar(&flagMatchStatus, "mc", "", "match status codes")
	scanCmd.Flags().StringVar(&flagFilterStatus, "fc", "", "filter status codes")
	scanCmd.Flags().StringVar(&flagMatchSize, "ms", "", "match size")
	scanCmd.Flags().StringVar(&flagFilterSize, "fs", "", "filter size")
	scanCmd.Flags().StringSliceVar(&flagMatchRegex, "mr", nil, "match regex")
	scanCmd.Flags().StringSliceVar(&flagFilterRegex, "fr", nil, "filter regex")
	scanCmd.Flags().StringSliceVar(&flagMatchContent, "mt", nil, "match content types")
	scanCmd.Flags().StringSliceVar(&flagFilterContent, "ft", nil, "filter content types")
}

func validateHeaderFlag(val string) error {
	idx := strings.Index(val, "=")
	if idx <= 0 || idx == len(val)-1 {
		return fmt.Errorf("header flag %q must be in Name=Value format", val)
	}
	return nil
}

func parseHeaderFlags(flags []string) []config.HeaderFilter {
	res := make([]config.HeaderFilter, 0, len(flags))
	for _, h := range flags {
		idx := strings.Index(h, "=")
		res = append(res, config.HeaderFilter{
			Name:  strings.TrimSpace(h[:idx]),
			Value: strings.TrimSpace(h[idx+1:]),
		})
	}
	return res
}

func mapHeaders(filters []config.HeaderFilter) []engine.HeaderFilter {
	out := make([]engine.HeaderFilter, len(filters))
	for i, f := range filters {
		out[i] = engine.HeaderFilter{
			Name:  f.Name,
			Value: f.Value,
		}
	}
	return out
}

// shouldEnableProgress returns true when the live progress renderer should be
// activated automatically. Progress is suppressed when:
//   - --no-progress was explicitly requested
//   - --quiet mode is active
//   - output format is json or ndjson
//   - stdout is not a terminal (piped, redirected, or CI environment)
func shouldEnableProgress(cfg config.Config, noProgress bool) bool {
	if noProgress {
		return false
	}
	if cfg.Quiet {
		return false
	}
	if cfg.OutputFormat == "json" || cfg.OutputFormat == "ndjson" || cfg.OutputFormat == "csv" || cfg.OutputFormat == "markdown" {
		return false
	}
	if !console.IsTerminal(os.Stdout.Fd()) {
		return false
	}
	return true
}

func parseCookies(cookieStr string) []*http.Cookie {
	if cookieStr == "" {
		return nil
	}
	header := http.Header{"Cookie": []string{cookieStr}}
	req := &http.Request{Header: header}
	return req.Cookies()
}
