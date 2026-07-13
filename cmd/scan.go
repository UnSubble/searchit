package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/adaptive"
	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/console"
	"github.com/unsubble/searchit/internal/engine"
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
	flagOutput          string
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

	resolvedTargets []string

	testHookConfigApplied func(config.Config)
)

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

		if flagOutput != "text" && flagOutput != "json" && flagOutput != "ndjson" {
			return fmt.Errorf("invalid output format %q: must be one of text, json, ndjson", flagOutput)
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
			if _, ok := adaptive.Lookup(flagTech); !ok {
				return fmt.Errorf(
					"unknown technology %q; supported values: %s",
					flagTech, adaptive.SupportedIDs(),
				)
			}
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
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

		if cfg.Output == "text" && !cfg.Quiet && len(appliedProfiles) > 0 {
			fmt.Println("[*] Profiles:")
			for _, name := range appliedProfiles {
				fmt.Printf("    %s\n", name)
			}
		}
		if cfg.Output == "text" && !cfg.Quiet && cfg.TechProfile != nil {
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

		var fmttr output.Formatter
		switch cfg.Output {
		case "json":
			fmttr = output.NewJSONFormatter(os.Stdout)
		case "ndjson":
			fmttr = output.NewNDJSONFormatter(os.Stdout)
		default:
			fmttr = output.NewTextFormatter(os.Stdout, cfg.Quiet)
		}
		defer fmttr.Close()

		var limiter *rate.Limiter
		if cfg.Rate > 0 {
			limiter = rate.NewLimiter(rate.Limit(cfg.Rate), 1)
		}

		for _, targetURL := range cfg.URLs {
			if cfg.Output == "text" {
				fmt.Printf("[*] Target: %s\n", targetURL)
			}

			scanCtx, scanCancel := context.WithCancel(ctx)
			collector := stats.NewCollector()
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
				if cfg.Output == "text" {
					fmt.Println("[*] Recursive scan enabled")
					fmt.Printf("[*] Strategy: %s\n", cfg.Strategy.String())
					fmt.Printf("[*] Max depth: %d\n", cfg.MaxDepth)
					fmt.Printf("[*] Recurse on: %s\n", formatRecurseOn(cfg))
					if !cfg.RecurseOn.Match(404) {
						fmt.Println("[*] 404 responses are ignored by default.")
					}
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
					appState.FingerprintCache,
				)
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
					appState.Config.Status.Exclude,
					cfg.IncludeSize,
					cfg.ExcludeSize,
					mapHeaders(cfg.IncludeHeaders),
					mapHeaders(cfg.ExcludeHeaders),
					cfg.Threads,
					cfg.Delay,
					limiter,
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
					if r.Accepted {
						if progMgr != nil {
							progMgr.HandleResult(r)
						} else {
							_ = fmttr.Print(r)
						}
					}
				}
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
	if cmd.Flags().Changed("exclude-status") {
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
	if cmd.Flags().Changed("output") {
		cfg.Output = flagOutput
	}
	if cmd.Flags().Changed("quiet") {
		cfg.Quiet = flagQuiet
	}
	if cmd.Flags().Changed("include-size") {
		if f, err := size.Parse(flagIncludeSize); err == nil {
			cfg.IncludeSize = f
		}
	}
	if cmd.Flags().Changed("exclude-size") {
		if f, err := size.Parse(flagExcludeSize); err == nil {
			cfg.ExcludeSize = f
		}
	}
	if cmd.Flags().Changed("include-header") {
		cfg.IncludeHeaders = parseHeaderFlags(flagIncludeHeaders)
	}
	if cmd.Flags().Changed("exclude-header") {
		cfg.ExcludeHeaders = parseHeaderFlags(flagExcludeHeaders)
	}
	if flagTech != "" {
		if p, ok := adaptive.Lookup(flagTech); ok {
			cfg.TechProfile = &p
		}
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
		"text",
		"output format (text, json, ndjson)",
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

	scanCmd.Flags().StringSliceVarP(
		&flagIncludeHeaders,
		"include-header",
		"H",
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
	if cfg.Output == "json" || cfg.Output == "ndjson" {
		return false
	}
	if !console.IsTerminal(os.Stdout.Fd()) {
		return false
	}
	return true
}
