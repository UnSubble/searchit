package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/console"
	"github.com/unsubble/searchit/internal/diagnostics"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/extensions"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/httpclient"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/output/telemetry"
	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/resolver"
	"github.com/unsubble/searchit/internal/progress"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/signals"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/state"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/targets"
	"github.com/unsubble/searchit/internal/useragent"
	"github.com/unsubble/searchit/internal/wordlist"
	"golang.org/x/time/rate"
)

type ScanOptions struct {
	URL             string
	URLFile         string
	Wordlist        string
	Ext             []string
	Threads         int
	Timeout         int
	Recursive       bool
	MaxDepth        uint16
	Strategy        string
	ExcludeStatus   string
	RecurseOn       string
	NormalizePaths  bool
	CollapseSlashes bool
	Output          string
	Format          string
	Quiet           bool
	IncludeSize     string
	ExcludeSize     string
	IncludeHeaders  []string
	ExcludeHeaders  []string
	Delay           string
	Rate            float64
	ConnectTimeout  string
	Profiles        []string
	RawProfile      string
	NoProgress      bool
	Tech            string
	FollowRedirects bool
	MaxRedirects    int
	Adaptive        bool

	Method      string
	HTTPVersion string
	Data        string
	Headers     []string
	Cookie      string
	Proxy       string
	UserAgent   string
	RandomAgent bool

	MatchStatus   string
	FilterStatus  string
	MatchSize     string
	FilterSize    string
	MatchRegex    []string
	FilterRegex   []string
	MatchContent  []string
	FilterContent []string
	ShowHeaders   bool
	ShowTitle     bool
	Request       string
	HelpAll       bool

	resolvedTargets       []targets.Target
	testHookConfigApplied func(config.Config)
}

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

func NewScanCmd() (*cobra.Command, *ScanOptions) {
	opts := &ScanOptions{}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan a target URL",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && (args[0] == "help" || args[0] == "help-all") {
				if args[0] == "help-all" || (len(args) > 1 && args[1] == "all") {
					opts.HelpAll = true
				}
				return pflag.ErrHelp
			}
			if opts.HelpAll {
				return pflag.ErrHelp
			}
			if opts.RawProfile != "" {
				for _, p := range strings.Split(opts.RawProfile, ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						opts.Profiles = append(opts.Profiles, p)
					}
				}
			}

			if opts.Threads < 1 {
				return fmt.Errorf("threads must be at least 1")
			}
			if opts.MaxDepth < 1 {
				return fmt.Errorf("max depth must be at least 1")
			}
			if opts.Strategy != "bfs" && opts.Strategy != "dfs" && opts.Strategy != "priority" {
				return fmt.Errorf("invalid strategy %q: must be bfs, dfs, or priority", opts.Strategy)
			}
			if cmd.Flags().Changed("max-depth") && !opts.Recursive {
				return fmt.Errorf("max-depth requires recursive scanning to be enabled")
			}
			if cmd.Flags().Changed("strategy") && !opts.Recursive {
				return fmt.Errorf("strategy requires recursive scanning to be enabled")
			}
			if cmd.Flags().Changed("recurse-on") && !opts.Recursive {
				return fmt.Errorf("recurse-on requires recursive scanning to be enabled")
			}

			if opts.Recursive {
				if _, err := status.Parse(opts.RecurseOn); err != nil {
					return fmt.Errorf("invalid recurse-on: %w", err)
				}
			}

			// Validate --format if explicitly provided.
			if cmd.Flags().Changed("format") {
				if _, err := output.Parse(opts.Format); err != nil {
					return fmt.Errorf("invalid --format: %w", err)
				}
			}

			// --output is a file path; validate that it is not an existing directory.
			if opts.Output != "" {
				if fi, err := os.Stat(opts.Output); err == nil && fi.IsDir() {
					return fmt.Errorf("--output %q is a directory; provide a file path", opts.Output)
				}
			}

			if _, err := size.Parse(opts.IncludeSize); err != nil {
				return fmt.Errorf("invalid include-size: %w", err)
			}
			if _, err := size.Parse(opts.ExcludeSize); err != nil {
				return fmt.Errorf("invalid exclude-size: %w", err)
			}

			if opts.Delay != "" {
				if _, err := time.ParseDuration(opts.Delay); err != nil {
					return fmt.Errorf("invalid delay: %w", err)
				}
			}

			if cmd.Flags().Changed("rate") && opts.Rate <= 0 {
				return fmt.Errorf("rate must be greater than 0")
			}

			if opts.ConnectTimeout != "" {
				ct, err := time.ParseDuration(opts.ConnectTimeout)
				if err != nil {
					return fmt.Errorf("invalid connect-timeout: %w", err)
				}
				if ct < 0 {
					return fmt.Errorf("connect-timeout cannot be negative")
				}
			}

			if opts.HTTPVersion != "" {
				if err := httpclient.ValidateHTTPVersion(opts.HTTPVersion); err != nil {
					return err
				}
			}

			if opts.MaxRedirects < 0 {
				return fmt.Errorf("max-redirects cannot be negative")
			}

			for _, h := range opts.IncludeHeaders {
				if err := validateHeaderFlag(h); err != nil {
					return fmt.Errorf("invalid include-header: %w", err)
				}
			}
			for _, h := range opts.ExcludeHeaders {
				if err := validateHeaderFlag(h); err != nil {
					return fmt.Errorf("invalid exclude-header: %w", err)
				}
			}

			for _, h := range opts.Headers {
				idx := strings.Index(h, "=")
				if idx == -1 {
					idx = strings.Index(h, ":")
				}
				if idx <= 0 || idx == len(h)-1 {
					return fmt.Errorf("invalid header %q: must be in Name: Value or Name=Value format", h)
				}
			}

			if opts.Proxy != "" {
				// Try parsing as url
				if _, err := url.Parse(opts.Proxy); err != nil {
					return fmt.Errorf("invalid proxy URL %q: %w", opts.Proxy, err)
				}
			}

			if opts.MatchStatus != "" {
				if _, err := status.Parse(opts.MatchStatus); err != nil {
					return fmt.Errorf("invalid match-status (--mc): %w", err)
				}
			}
			if opts.FilterStatus != "" {
				if _, err := status.Parse(opts.FilterStatus); err != nil {
					return fmt.Errorf("invalid filter-status (--fc): %w", err)
				}
			}
			if opts.MatchSize != "" {
				if _, err := size.Parse(opts.MatchSize); err != nil {
					return fmt.Errorf("invalid match-size (--ms): %w", err)
				}
			}
			if opts.FilterSize != "" {
				if _, err := size.Parse(opts.FilterSize); err != nil {
					return fmt.Errorf("invalid filter-size (--fs): %w", err)
				}
			}
			for _, rx := range opts.MatchRegex {
				if _, err := regexp.Compile(rx); err != nil {
					return fmt.Errorf("invalid match-regex (--mr) %q: %w", rx, err)
				}
			}
			for _, rx := range opts.FilterRegex {
				if _, err := regexp.Compile(rx); err != nil {
					return fmt.Errorf("invalid filter-regex (--fr) %q: %w", rx, err)
				}
			}

			if len(opts.Ext) > 0 {
				if _, err := extensions.Parse(opts.Ext); err != nil {
					return fmt.Errorf("invalid --ext: %w", err)
				}
			}

			// Parse target URLs when they are provided via CLI flags.
			// When no URL flag is present but profiles are specified, defer target
			// resolution to RunE where profiles will have been applied and may
			// supply a url or url-file value.
			hasURL := opts.URL != "" || opts.URLFile != "" || opts.Request != ""
			if hasURL {
				var errParse error
				opts.resolvedTargets, errParse = targets.Parse(targets.ParseOptions{
					URL:         opts.URL,
					URLFile:     opts.URLFile,
					RequestFile: opts.Request,
				})
				if errParse != nil {
					return errParse
				}
			} else if len(opts.Profiles) == 0 {
				return fmt.Errorf("no target URL specified; use -u/--url, --url-file, or --profile with url/url-file config")
			}
			// else: profiles provided, no URL flag — defer to RunE.

			if opts.Tech != "" {
				if _, ok := lookupTech(opts.Tech); !ok {
					return fmt.Errorf(
						"unknown technology %q; supported values: %s",
						opts.Tech, supportedTechIDs(),
					)
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.HelpAll {
				return cmd.Help()
			}
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

			// 1. Resolve profiles (left → right).
			var profileOverlays []config.ScanOverlay
			var appliedProfiles []string
			if len(opts.Profiles) > 0 {
				store := profile.NewStore()

				for _, profileName := range opts.Profiles {
					res := resolver.New(store)
					resolved, err := res.Resolve([]string{profileName})
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

						// Decode into config.ScanOverlay
						var overlay config.ScanOverlay
						if err := p.Decode(&overlay); err != nil {
							cmd.SilenceErrors = true
							cmd.SilenceUsage = true
							fmt.Fprintf(cmd.ErrOrStderr(), "failed to load profile:\n%v\n", err)
							return fmt.Errorf("decode failed")
						}

						profileOverlays = append(profileOverlays, overlay)
					}
				}
			}

			// 2. Resolve baseline configuration (Defaults -> Global Config File -> Profiles).
			cfg, err := config.ResolveScanConfig(cfgFile, profileOverlays)
			if err != nil {
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
				fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
				return fmt.Errorf("config error")
			}

			// 3. Apply CLI flag overrides.
			applyCLIOverrides(opts, cmd, &cfg)

			// If no targets were resolved from CLI flags, attempt to populate them
			// from values supplied via profile (url: or url-file:).
			if len(opts.resolvedTargets) == 0 {
				if len(cfg.URLs) > 0 {
					for _, u := range cfg.URLs {
						opts.resolvedTargets = append(opts.resolvedTargets, targets.Target{URL: u})
					}
				} else if cfg.URLFile != "" {
					parsed, err := targets.Parse(targets.ParseOptions{URLFile: cfg.URLFile})
					if err != nil {
						return fmt.Errorf("profile url-file: %w", err)
					}
					opts.resolvedTargets = parsed
				}
			}
			if len(opts.resolvedTargets) == 0 {
				return fmt.Errorf("no target URL specified; use -u/--url, --url-file, or set url/url-file in the profile")
			}

			if cfg.RequestFile != "" && len(opts.resolvedTargets) > 0 {
				t := opts.resolvedTargets[0]
				cfg.Method = t.Method
				cfg.Data = t.Body

				if t.Cookies != "" {
					cfg.Cookies = t.Cookies
				}

				cfg.Headers = nil
				cfg.Headers = append(cfg.Headers, t.Headers...)

				cfg.URLs = []string{t.URL}
			}

			if !cfg.Quiet && len(appliedProfiles) > 0 {
				fmt.Fprintln(os.Stderr, "[*] Profiles:")
				for _, name := range appliedProfiles {
					fmt.Fprintf(os.Stderr, "    %s\n", name)
				}
			}
			if !cfg.Quiet && cfg.TechProfile != nil {
				fmt.Fprintf(os.Stderr, "[*] Tech profile: %s\n", cfg.TechProfile.ID)
			}

			if opts.testHookConfigApplied != nil {
				opts.testHookConfigApplied(cfg)
				return nil
			}

			stateMgr := state.NewManager()
			stateMgr.Transition(state.PhaseStarting)

			ctx := cmd.Context()
			ctx, cancelGraceful := context.WithCancel(ctx)
			defer cancelGraceful()

			var activeTargetCtx context.Context
			var activeTargetMu sync.Mutex

			drainCtx, cancelDrain := context.WithCancel(context.Background())
			defer cancelDrain()

			signals.SetupGlobal(drainCtx, func() {
				if ctx.Err() != nil {
					// Already gracefully shutting down. This SIGINT is a force abort.
					cancelDrain()
					return
				}
				activeTargetMu.Lock()
				tgtCtx := activeTargetCtx
				activeTargetMu.Unlock()
				if tgtCtx != nil && tgtCtx.Err() != nil {
					// The target is already draining (e.g. from 'q'). Force abort.
					cancelDrain()
					return
				}
				if stateMgr != nil && stateMgr.Current() < state.PhaseShutdownRequested {
					stateMgr.Transition(state.PhaseShutdownRequested)
				}
				cancelGraceful()
			}, func() {
				if stateMgr != nil && stateMgr.Current() < state.PhaseFinalizing {
					stateMgr.Transition(state.PhaseFinalizing)
				}
				cancelDrain()
			})

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
					if len(cfg.Extensions) > 0 {
						totalWords *= (1 + len(cfg.Extensions))
					}
				}
			}

			// Determine the output writer (file or stdout) for the formatter.
			var outWriter io.Writer = os.Stdout
			if cfg.OutputFile != "" {
				f, err := os.OpenFile(cfg.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
				if err != nil {
					return fmt.Errorf("cannot open output file: %w", err)
				}
				defer f.Close()
				outWriter = f
			}

			httpclient.ConfigureTransportForWorkers(appState.HTTPClient, cfg.Threads)

			// Construct the formatter from the resolved format name.
			fmt_, err := output.Parse(cfg.OutputFormat)
			if err != nil {
				// Fallback — should not happen after validation, but be safe.
				fmt_ = output.FormatText
			}

			var limiter *rate.Limiter
			if cfg.Rate > 0 {
				limiter = rate.NewLimiter(rate.Limit(cfg.Rate), 1)
			}

			customHeaders, err := parseFuzzHeaderFlags(cfg.Headers)
			if err != nil {
				return err
			}

			// Resolve and inject User-Agent once before any request is made.
			// Resolution order: -H "User-Agent=..." > --user-agent > profile/--random-agent.
			var randomUA string
			if cfg.RandomAgent {
				randomUA = useragent.Random()
			}
			if ua := useragent.Resolve(customHeaders.Get("User-Agent"), cfg.UserAgent, randomUA); ua != "" {
				customHeaders.Set("User-Agent", ua)
			}

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
			fs.ShowHeaders = cfg.ShowHeaders
			fs.ShowTitle = cfg.ShowTitle

			var globalFmttr output.Formatter
			if cfg.OutputFormat != "text" {
				globalFmttr = output.New(fmt_, outWriter, cfg.Quiet, cfg.ShowHeaders, cfg.ShowTitle)
				if globalFmttr != nil {
					defer globalFmttr.Close()
				}
			}

			targetManager := targets.NewManager(opts.resolvedTargets)
			globalSummary := targets.NewGlobalSummary(len(opts.resolvedTargets))

			err = targetManager.Execute(ctx, func(tCtx targets.TargetContext) error {
				activeTargetMu.Lock()
				activeTargetCtx = tCtx.Ctx
				activeTargetMu.Unlock()

				var infoLog io.Writer
				if !cfg.Quiet {
					infoLog = cmd.ErrOrStderr()
				}
				resolvedURL, err := targets.AutoDetectTarget(tCtx.Ctx, appState.HTTPClient, tCtx.Target.URL, infoLog)
				if err != nil {
					return err
				}
				tCtx.Target.URL = resolvedURL
				targetURL := resolvedURL
				scanCtx := tCtx.Ctx
				scanCancel := tCtx.Cancel
				// ensure cancel is invoked (though manager also cleans up)
				defer scanCancel()

				// 1. Create a fresh TerminalManager for this target's lifecycle (writing to stderr).
				tm := terminal.New(os.Stderr)
				if err := tm.AcquireOwner(terminal.OwnerConfiguration); err != nil {
					return err
				}

				// 2. Formatter setup.
				var fmttr output.Formatter
				if globalFmttr != nil {
					fmttr = globalFmttr
				} else {
					fmttr = output.New(fmt_, outWriter, cfg.Quiet, cfg.ShowHeaders, cfg.ShowTitle)
				}

				if !cfg.Quiet {
					primaryWl := cfg.Wordlist
					if primaryWl == "" {
						primaryWl = "embedded"
					}
					modeStr := "Standard"
					if cfg.Recursive {
						modeStr = "Recursive"
					}

					traversalStr := ""
					if cfg.Recursive {
						traversalStr = strings.ToUpper(cfg.Strategy.String())
					}

					excludeStatusStr := cfg.Status.Exclude.String()
					if excludeStatusStr == "" {
						excludeStatusStr = "none"
					}

					info := telemetry.ConfigInfo{
						Target:          targetURL,
						Method:          cfg.Method,
						Workers:         cfg.Threads,
						Mode:            modeStr,
						Traversal:       traversalStr,
						AdaptiveEnabled: cfg.Adaptive,
						WordlistsCount:  1,
						PrimaryWordlist: primaryWl,
						HTTPVersion:     "auto",
						FollowRedirects: cfg.FollowRedirects,
						FilterStatus:    excludeStatusStr,
						TotalCandidates: totalWords,
						IsFuzz:          false,
						Extensions:      cfg.Extensions,
					}
					telemetry.PrintConfiguration(tm, terminal.OwnerConfiguration, info)
				}

				// Transition out of Starting, into Running, and hand over to Progress.
				if err := tm.TransitionAndRelease(terminal.PhaseRunning, terminal.OwnerConfiguration); err != nil {
					return err
				}
				if err := tm.AcquireOwner(terminal.OwnerProgress); err != nil {
					return err
				}

				entriesPerDir := int64(totalWords)
				totalWork := entriesPerDir
				if cfg.Recursive {
					maxDepth := int64(cfg.MaxDepth)
					if maxDepth < 1 {
						maxDepth = 1
					}
					totalWork = entriesPerDir * maxDepth
				}

				collector := stats.NewCollector()
				if cfg.Recursive {
					collector.SetIsFinite(false)
				} else {
					collector.SetIsFinite(true)
					if totalWork > 0 {
						collector.SetTotalWork(totalWork)
					}
				}
				var progMgr *progress.Manager
				var renderer *progress.ANSIRenderer
				var progCmdChan chan console.Command
				var consoleCtrl *console.Controller
				var termCtx context.Context
				var cancelTerm context.CancelFunc

				enableProgress := shouldEnableProgress(cfg, opts.NoProgress)
				// Interactive keyboard controls require stdin to also be a terminal.
				interactive := enableProgress && console.IsTerminal(os.Stdin.Fd())

				var progDone chan struct{}
				progCtx, cancelProg := context.WithCancel(scanCtx)
				defer cancelProg()

				if enableProgress {
					termCtx, cancelTerm = context.WithCancel(scanCtx)
					defer cancelTerm()
					modeStr := "Single target"
					if cfg.Recursive {
						modeStr = fmt.Sprintf("Recursive (%s)", strings.ToUpper(cfg.Strategy.String()))
					}

					renderer = progress.NewANSIRenderer(tm, targetURL, appliedProfiles, modeStr)
					renderer.Method = cfg.Method
					renderer.HTTPVersion = "HTTP/1.1"
					renderer.IsPaused = func() bool {
						return stateMgr != nil && stateMgr.Current() == state.PhasePaused
					}
					progMgr = progress.NewManager(tm, collector, renderer, 1*time.Second)
					progMgr.ConfiguredThreads = cfg.Threads

					if interactive {
						consoleCtrl = console.NewController(os.Stdin)
						progCmdChan = make(chan console.Command, 10)
						go consoleCtrl.Start(termCtx)

						go func() {
							for cmd := range consoleCtrl.Commands() {
								switch cmd {
								case console.CommandProgress, console.CommandStats:
									select {
									case progCmdChan <- cmd:
									default:
									}
								case console.CommandPauseToggle:
									if stateMgr != nil {
										if stateMgr.Current() == state.PhaseRunning {
											stateMgr.Transition(state.PhasePaused)
										} else if stateMgr.Current() == state.PhasePaused {
											stateMgr.Transition(state.PhaseRunning)
										}
									}
									select {
									case progCmdChan <- console.CommandProgress:
									default:
									}
								case console.CommandStopTarget:
									scanCancel()
								case console.CommandAbortAll:
									if scanCtx.Err() != nil {
										cancelDrain()
									} else {
										if stateMgr != nil && stateMgr.Current() < state.PhaseShutdownRequested {
											stateMgr.Transition(state.PhaseShutdownRequested)
										}
										cancelGraceful()
									}
								}
							}
							close(progCmdChan)
						}()
					}

					progDone = make(chan struct{})
					go func() {
						defer close(progDone)
						progMgr.Start(progCtx, progCmdChan)
					}()
				}

				// Dedicated shutdown coordinator for terminal and renderer
				shutdownDone := make(chan struct{})
				go func() {
					defer close(shutdownDone)
					<-scanCtx.Done()

					if stateMgr != nil && stateMgr.Current() < state.PhaseShutdownRequested {
						stateMgr.Transition(state.PhaseShutdownRequested)
					}

					if cancelTerm != nil {
						cancelTerm()
						if consoleCtrl != nil {
							<-consoleCtrl.Done()
						}
					}
					if cancelProg != nil {
						cancelProg()
					}
					if progDone != nil {
						<-progDone
					}

					if enableProgress && renderer != nil {
						_ = renderer.Close(terminal.OwnerProgress)
						if stateMgr.Current() < state.PhaseFinalizing {
							_ = tm.Emit(terminal.OwnerProgress, func(w io.Writer) {
								fmt.Fprintln(w, "\r\n[*] Draining in-flight requests...")
							})
						}
					}

					// Diagnostic: dump structured state if shutdown takes too long
					diagTimeout := cfg.Timeout + 2*time.Second
					if diagTimeout < 5*time.Second {
						diagTimeout = 5 * time.Second
					}
					go diagnostics.RunDiagnostics(diagTimeout, scanCtx.Err(), drainCtx.Err())
				}()

				stateMgr.Transition(state.PhaseRunning)
				var manager *recursion.Manager
				if cfg.Recursive {

					var fpCache *fingerprint.Cache
					if cfg.Adaptive {
						fpCache = appState.FingerprintCache
					}

					seeds := []string{targetURL}
					manager = recursion.NewManager(
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
						entriesPerDir,
					)
					if appState.AdaptiveEngine != nil {
						manager.SetAdaptiveEngine(appState.AdaptiveEngine)
					}
					manager.SetRequestManipulation(cfg.Method, []byte(cfg.Data), customHeaders, cfg.Cookies)
					manager.SetFilterSuite(fs)
					manager.SetStats(collector)
					manager.SetExtensions(cfg.Extensions)
					manager.PauseBlocker = stateMgr.WaitUntilRunning
					manager.Run(scanCtx, drainCtx, seeds, cfg.Threads, func(r engine.Result) {
						if r.Accepted {
							if outWriter == os.Stdout && progMgr != nil {
								progMgr.ExecuteAbove(func() {
									_ = fmttr.Print(r)
								})
							} else {
								_ = fmttr.Print(r)
							}
						} else if r.Err != nil {
							errStr := r.Err.Error()
							if strings.Contains(errStr, "maximum redirect limit exceeded") {
								fmt.Fprintln(os.Stderr, "ERROR: maximum redirect limit exceeded")
							} else if strings.Contains(errStr, "redirect loop detected") {
								fmt.Fprintln(os.Stderr, "ERROR: redirect loop detected")
							}
						}
					})
				} else {
					jobs := make(chan engine.Job, cfg.Threads)
					results := engine.Start(
						scanCtx,
						drainCtx,
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
						cfg.Cookies,
						jobs,
						collector,
						stateMgr.WaitUntilRunning,
						engine.WorkerOptions{ExtractLinks: false},
					)

					go func() {
						var skipSet sync.Map

						if appState.AdaptiveEngine != nil {
							_ = appState.AdaptiveEngine.Discover(scanCtx)
							discovered := appState.AdaptiveEngine.GetDiscoveredJobs()
							for _, j := range discovered {
								normKey := strings.TrimRight(strings.ToLower(j.URL), "/")
								if _, loaded := skipSet.LoadOrStore(normKey, true); !loaded {
									select {
									case <-scanCtx.Done():
										close(jobs)
										return
									case jobs <- j:
										atomic.AddInt64(&stats.GlobalInstrumentation.JobsSubmitted, 1)
										if collector != nil {
											collector.RecordJobProduced()
											collector.AddTotalCandidates(1)
										}
									}
								}
							}
						}

						p := wordlist.Producer{
							BaseURL:         targetURL,
							Reader:          reader,
							NormalizePaths:  cfg.Paths.NormalizePaths,
							CollapseSlashes: cfg.Paths.CollapseSlashes,
							Extensions:      cfg.Extensions,
							Collector:       collector,
							PauseBlocker:    stateMgr.WaitUntilRunning,
							SkipSet:         &skipSet,
						}
						_ = p.Produce(scanCtx, jobs)
					}()

					for r := range results {
						atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
						if r.Accepted {
							atomic.AddInt64(&stats.GlobalInstrumentation.ResultsAccepted, 1)
							if outWriter == os.Stdout && progMgr != nil {
								progMgr.ExecuteAbove(func() {
									_ = fmttr.Print(r)
								})
							} else {
								_ = fmttr.Print(r)
							}
						} else {
							atomic.AddInt64(&stats.GlobalInstrumentation.ResultsRejected, 1)
							if r.Err != nil {
								errStr := r.Err.Error()
								if strings.Contains(errStr, "maximum redirect limit exceeded") {
									fmt.Fprintln(os.Stderr, "ERROR: maximum redirect limit exceeded")
								} else if strings.Contains(errStr, "redirect loop detected") {
									fmt.Fprintln(os.Stderr, "ERROR: redirect loop detected")
								}
							}
						}
					}
				}

				if fmttr != nil && globalFmttr == nil {
					_ = fmttr.Close()
				}

				// In case the target naturally completed, trigger shutdown done.
				scanCancel()
				<-shutdownDone

				stateMgr.Transition(state.PhaseWaitingWorkers)
				_ = tm.TransitionTo(terminal.PhaseWaitingWorkers)

				stateMgr.Transition(state.PhaseFinalizing)
				_ = tm.TransitionTo(terminal.PhaseFinalizing)

				stateMgr.Transition(state.PhaseTerminalShutdown)
				_ = tm.TransitionTo(terminal.PhaseTerminalShutdown)

				if enableProgress && renderer != nil {
					_ = renderer.Close(terminal.OwnerProgress) // ensure it's closed in case shutdown coordinator didn't
				}
				_ = tm.ReleaseOwner(terminal.OwnerProgress)

				stateMgr.Transition(state.PhaseSummary)
				_ = tm.AcquireAndTransition(terminal.OwnerSummary, terminal.PhaseSummary)

				if !cfg.Quiet {
					snap := collector.Snapshot()
					modeStr := "Standard"
					if cfg.Recursive {
						modeStr = "Recursive"
					}

					traversalStr := ""
					if cfg.Recursive {
						traversalStr = strings.ToUpper(cfg.Strategy.String())
					}

					telemetry.PrintSummary(tm, terminal.OwnerSummary, telemetry.SummaryInfo{
						IsFuzz:          false,
						Mode:            modeStr,
						Traversal:       traversalStr,
						AdaptiveEnabled: cfg.Adaptive,
						Findings:        int(snap.Discovered),
						Snapshot:        snap,
					}, flagDebug)
					_ = tm.TransitionAndRelease(terminal.PhasePipeline, terminal.OwnerSummary)

					stateMgr.Transition(state.PhasePipeline)
					_ = tm.AcquireOwner(terminal.OwnerPipeline)

					if flagDebug {
						if appState.AdaptiveEngine != nil {
							techs, discoveries, high, med, low := appState.AdaptiveEngine.GetMetrics()
							dfsCount, bfsCount, eagerCount := 0, 0, 0
							if manager != nil {
								dfsCount = manager.DFSCount
								bfsCount = manager.BFSCount
								eagerCount = manager.EagerCount
							}
							telemetry.PrintAdaptive(tm, terminal.OwnerPipeline, telemetry.AdaptiveInfo{
								Technologies:        techs,
								Discoveries:         discoveries,
								DFSCount:            dfsCount,
								BFSCount:            bfsCount,
								EagerCount:          eagerCount,
								HighPriorityCount:   high,
								MediumPriorityCount: med,
								LowPriorityCount:    low,
							})
						}

						telemetry.PrintPipelineReconciliation(tm, terminal.OwnerPipeline)
					}
					_ = tm.TransitionAndRelease(terminal.PhaseDone, terminal.OwnerPipeline)
				} else {
					// Quiet mode bypasses the owners but we still must enforce the TM terminal state.
					_ = tm.TransitionTo(terminal.PhasePipeline)
					stateMgr.Transition(state.PhasePipeline)
					_ = tm.TransitionTo(terminal.PhaseDone)
				}

				stateMgr.Transition(state.PhaseDone)
				scanCancel()

				// Accumulate snapshot into global summary
				globalSummary.AddSnapshot(collector.Snapshot())

				return nil
			})

			if err != nil && !errors.Is(err, context.Canceled) {
				return err
			}

			// Optionally print global summary if multiple targets
			if len(opts.resolvedTargets) > 1 && !cfg.Quiet {
				fmt.Fprintf(os.Stderr, "\n[*] Global Summary:\n    Targets scanned: %d/%d\n    Total Requests: %d\n    Total Discoveries: %d\n    Duration: %s\n",
					globalSummary.TargetsRun, globalSummary.TargetsTotal, globalSummary.TotalJobs, globalSummary.TotalFound, globalSummary.Duration())
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(
		&opts.URL,
		"url",
		"u",
		"",
		"target URL",
	)

	cmd.Flags().StringVarP(
		&opts.Wordlist,
		"wordlist",
		"w",
		"",
		"wordlist path",
	)

	cmd.Flags().IntVarP(
		&opts.Threads,
		"threads",
		"t",
		32,
		"number of concurrent workers",
	)

	cmd.Flags().IntVar(
		&opts.Timeout,
		"timeout",
		10,
		"timeout in seconds",
	)

	cmd.Flags().StringSliceVarP(
		&opts.Ext,
		"ext",
		"e",
		nil,
		"comma-separated extensions or @file",
	)

	cmd.Flags().BoolVarP(
		&opts.Recursive,
		"recursive",
		"r",
		false,
		"enable recursive directory scanning",
	)

	cmd.Flags().Uint16VarP(
		&opts.MaxDepth,
		"max-depth",
		"d",
		3,
		"maximum recursion depth",
	)

	cmd.Flags().StringVarP(
		&opts.Strategy,
		"strategy",
		"s",
		"bfs",
		"recursion strategy (bfs, dfs, priority)",
	)

	cmd.Flags().StringVarP(
		&opts.ExcludeStatus,
		"exclude-status",
		"x",
		"404",
		"comma-separated status codes to exclude",
	)

	cmd.Flags().StringVar(
		&opts.RecurseOn,
		"recurse-on",
		"200,301,302,403",
		"comma-separated status codes to recurse on",
	)

	cmd.Flags().BoolVar(
		&opts.NormalizePaths,
		"normalize-paths",
		false,
		"normalize relative segments in paths (e.g. ././admin -> admin)",
	)

	cmd.Flags().BoolVar(
		&opts.CollapseSlashes,
		"collapse-slashes",
		false,
		"collapse consecutive slashes (e.g. admin////api -> admin/api)",
	)

	cmd.Flags().StringVarP(
		&opts.Output,
		"output",
		"o",
		"",
		"write results to this file (default: stdout); format auto-detected from extension",
	)

	cmd.Flags().StringVar(
		&opts.Format,
		"format",
		"text",
		fmt.Sprintf("output format (%s)", strings.Join(output.SupportedFormats(), ", ")),
	)

	cmd.Flags().StringVar(
		&opts.IncludeSize,
		"include-size",
		"",
		"comma-separated content sizes to include (e.g. 100-200,512)",
	)

	cmd.Flags().StringVar(
		&opts.ExcludeSize,
		"exclude-size",
		"",
		"comma-separated content sizes to exclude (e.g. 0,123)",
	)

	cmd.Flags().StringSliceVar(
		&opts.IncludeHeaders,
		"include-header",
		nil,
		"HTTP headers to include (e.g. Server=nginx)",
	)

	cmd.Flags().StringSliceVar(
		&opts.ExcludeHeaders,
		"exclude-header",
		nil,
		"HTTP headers to exclude (e.g. Content-Type=text/plain)",
	)

	cmd.Flags().BoolVarP(
		&opts.Quiet,
		"quiet",
		"q",
		false,
		"print only discovered URLs in text mode",
	)

	cmd.Flags().StringVar(
		&opts.URLFile,
		"url-file",
		"",
		"load targets from a file (one URL per line)",
	)

	cmd.Flags().StringVar(
		&opts.Delay,
		"delay",
		"",
		"delay between requests per worker",
	)

	cmd.Flags().Float64Var(
		&opts.Rate,
		"rate",
		0,
		"maximum requests per second across all workers",
	)

	cmd.Flags().StringVar(
		&opts.ConnectTimeout,
		"connect-timeout",
		"3s",
		"timeout for establishing new TCP connections",
	)

	cmd.Flags().StringVarP(
		&opts.RawProfile,
		"profile",
		"p",
		"",
		"apply one or more profiles (comma-separated)",
	)

	cmd.Flags().BoolVar(
		&opts.NoProgress,
		"no-progress",
		false,
		"disable the live progress display (progress is enabled automatically when stdout is a terminal)",
	)

	cmd.Flags().StringVar(
		&opts.Tech,
		"tech",
		"",
		"explicitly select a technology profile, bypassing automatic detection (e.g. laravel, spring, wordpress)",
	)

	cmd.Flags().BoolVar(
		&opts.FollowRedirects,
		"follow-redirects",
		false,
		"follow HTTP redirects",
	)

	cmd.Flags().IntVar(
		&opts.MaxRedirects,
		"max-redirects",
		10,
		"maximum redirect limit",
	)

	cmd.Flags().BoolVar(
		&opts.Adaptive,
		"adaptive",
		false,
		"enable adaptive scanning (technology detection and path injection)",
	)

	cmd.Flags().StringVarP(&opts.Method, "method", "X", "GET", "HTTP method to use for requests")
	cmd.Flags().StringVar(&opts.HTTPVersion, "http-version", "auto", "Select the HTTP protocol version (auto, 0.9, 1.0, 1.1, 2)")
	cmd.Flags().StringVar(&opts.Data, "data", "", "POST data body to use for requests")
	cmd.Flags().StringSliceVarP(&opts.Headers, "header", "H", nil, "HTTP request headers to send (e.g. -H 'Authorization: Bearer X')")
	cmd.Flags().StringVarP(&opts.Cookie, "cookie", "b", "", "HTTP request cookies to send (e.g. -b 'session=123')")
	cmd.Flags().StringVar(&opts.Proxy, "proxy", "", "HTTP proxy URL to use for requests")

	cmd.Flags().StringVar(&opts.MatchStatus, "mc", "", "match status codes")
	cmd.Flags().StringVar(&opts.FilterStatus, "fc", "", "filter status codes")
	cmd.Flags().StringVar(&opts.MatchSize, "ms", "", "match size")
	cmd.Flags().StringVar(&opts.FilterSize, "fs", "", "filter size")
	cmd.Flags().StringSliceVar(&opts.MatchRegex, "mr", nil, "match regex")
	cmd.Flags().StringSliceVar(&opts.FilterRegex, "fr", nil, "filter regex")
	cmd.Flags().StringSliceVar(&opts.MatchContent, "mt", nil, "match content types")
	cmd.Flags().StringSliceVar(&opts.FilterContent, "ft", nil, "filter content types")
	cmd.Flags().BoolVar(&opts.ShowHeaders, "show-headers", false, "show response headers in output")
	cmd.Flags().BoolVar(&opts.ShowTitle, "show-title", false, "show HTML titles in output")
	cmd.Flags().StringVar(&opts.Request, "request", "", "load raw HTTP request template from file")
	cmd.Flags().StringVar(&opts.UserAgent, "user-agent", "", "set a custom User-Agent for every request")
	cmd.Flags().BoolVar(&opts.RandomAgent, "random-agent", false, "use a randomly selected built-in User-Agent")
	cmd.Flags().BoolVar(&opts.HelpAll, "help-all", false, "show all available options")

	setupCmdHelp(cmd, func() bool { return opts.HelpAll }, scanHelpConfig)
	return cmd, opts
}

var scanHelpConfig = HelpConfig{
	Examples: `  searchit scan -u example.com -w raft-small.txt
  searchit scan -u example.com -w raft-small.txt -r
  searchit scan -u example.com --adaptive
  searchit scan -u example.com -o results.json`,
	Groups: []FlagGroup{
		{
			Title: "General",
			Names: []string{"url", "url-file", "wordlist"},
		},
		{
			Title: "Discovery",
			Names: []string{"recursive", "strategy", "adaptive", "ext", "profile"},
		},
		{
			Title: "HTTP",
			Names: []string{"method", "cookie", "data", "header"},
		},
		{
			Title: "Matching / Filtering",
			Names: []string{"mc", "ms", "mr", "fc", "fs", "fr"},
		},
		{
			Title: "Performance",
			Names: []string{"threads", "delay"},
		},
		{
			Title: "Output",
			Names: []string{"output", "quiet", "random-agent"},
		},
	},
	HelpAllCmd: "searchit scan --help-all",
}

// applyCLIOverrides applies CLI flag values to cfg, but only for flags
// that the user explicitly provided. This ensures that profile values
// are not overridden by default flag values.
func applyCLIOverrides(opts *ScanOptions, cmd *cobra.Command, cfg *config.Config) {
	var urls []string
	for _, t := range opts.resolvedTargets {
		urls = append(urls, t.URL)
	}
	cfg.URLs = urls

	if cmd.Flags().Changed("wordlist") {
		cfg.Wordlist = opts.Wordlist
	}
	if cmd.Flags().Changed("ext") {
		if exts, err := extensions.Parse(opts.Ext); err == nil {
			cfg.Extensions = exts
		}
	}
	if cmd.Flags().Changed("threads") {
		cfg.Threads = opts.Threads
	}
	if cmd.Flags().Changed("timeout") {
		cfg.Timeout = time.Duration(opts.Timeout) * time.Second
	}
	if cmd.Flags().Changed("delay") {
		if d, err := time.ParseDuration(opts.Delay); err == nil {
			cfg.Delay = d
		}
	}
	if cmd.Flags().Changed("rate") {
		cfg.Rate = opts.Rate
	}
	if cmd.Flags().Changed("connect-timeout") {
		if d, err := time.ParseDuration(opts.ConnectTimeout); err == nil {
			cfg.ConnectTimeout = d
		}
	}
	if cmd.Flags().Changed("recursive") {
		cfg.Recursive = opts.Recursive
	}
	if cmd.Flags().Changed("max-depth") {
		cfg.MaxDepth = opts.MaxDepth
	}
	if cmd.Flags().Changed("strategy") {
		if s, err := recursion.ParseStrategy(opts.Strategy); err == nil {
			cfg.Strategy = s
		}
	}
	if cmd.Flags().Changed("mc") {
		if f, err := status.Parse(opts.MatchStatus); err == nil {
			cfg.Status.Include = f
		}
	}
	if cmd.Flags().Changed("fc") {
		if f, err := status.Parse(opts.FilterStatus); err == nil {
			cfg.Status.Exclude = f
		}
	} else if cmd.Flags().Changed("exclude-status") {
		if f, err := status.Parse(opts.ExcludeStatus); err == nil {
			cfg.Status.Exclude = f
		}
	}
	if cmd.Flags().Changed("recurse-on") {
		if f, err := status.Parse(opts.RecurseOn); err == nil {
			cfg.RecurseOn = f
		}
	}
	if cmd.Flags().Changed("normalize-paths") {
		cfg.Paths.NormalizePaths = opts.NormalizePaths
	}
	if cmd.Flags().Changed("collapse-slashes") {
		cfg.Paths.CollapseSlashes = opts.CollapseSlashes
	}
	// --output is the output file path.
	if cmd.Flags().Changed("output") {
		cfg.OutputFile = opts.Output
	}
	// --format is the explicit formatter name.
	// Precedence: explicit --format > auto-detect from extension > default (text).
	if cmd.Flags().Changed("format") {
		if f, err := output.Parse(opts.Format); err == nil {
			cfg.OutputFormat = string(f)
		}
	} else if cfg.OutputFile != "" && !cmd.Flags().Changed("format") {
		// Auto-detect format from file extension when no explicit format is given.
		detected := output.FormatFromPath(cfg.OutputFile)
		cfg.OutputFormat = string(detected)
	}
	if cmd.Flags().Changed("quiet") {
		cfg.Quiet = opts.Quiet
	}
	if cmd.Flags().Changed("follow-redirects") {
		cfg.FollowRedirects = opts.FollowRedirects
	}
	if cmd.Flags().Changed("max-redirects") {
		cfg.MaxRedirects = opts.MaxRedirects
	}
	if cmd.Flags().Changed("ms") {
		if f, err := size.Parse(opts.MatchSize); err == nil {
			cfg.IncludeSize = f
		}
	} else if cmd.Flags().Changed("include-size") {
		if f, err := size.Parse(opts.IncludeSize); err == nil {
			cfg.IncludeSize = f
		}
	}
	if cmd.Flags().Changed("fs") {
		if f, err := size.Parse(opts.FilterSize); err == nil {
			cfg.ExcludeSize = f
		}
	} else if cmd.Flags().Changed("exclude-size") {
		if f, err := size.Parse(opts.ExcludeSize); err == nil {
			cfg.ExcludeSize = f
		}
	}
	if cmd.Flags().Changed("mr") {
		cfg.MatchRegex = opts.MatchRegex
	}
	if cmd.Flags().Changed("fr") {
		cfg.FilterRegex = opts.FilterRegex
	}
	if cmd.Flags().Changed("mt") {
		cfg.MatchContent = opts.MatchContent
	}
	if cmd.Flags().Changed("ft") {
		cfg.FilterContent = opts.FilterContent
	}
	if cmd.Flags().Changed("include-header") {
		cfg.IncludeHeaders = parseHeaderFlags(opts.IncludeHeaders)
	}
	if cmd.Flags().Changed("exclude-header") {
		cfg.ExcludeHeaders = parseHeaderFlags(opts.ExcludeHeaders)
	}
	if cmd.Flags().Changed("method") {
		cfg.Method = opts.Method
	}
	if cmd.Flags().Changed("http-version") {
		cfg.HTTPVersion = opts.HTTPVersion
	} else if cfg.HTTPVersion == "" {
		cfg.HTTPVersion = "auto"
	}
	if cmd.Flags().Changed("data") {
		cfg.Data = opts.Data
	}
	if cmd.Flags().Changed("header") {
		cfg.Headers = opts.Headers
	}
	if cmd.Flags().Changed("cookie") {
		cfg.Cookies = opts.Cookie
	}
	if cmd.Flags().Changed("proxy") {
		cfg.Proxy = opts.Proxy
	}
	if opts.Tech != "" {
		if p, ok := lookupTech(opts.Tech); ok {
			cfg.TechProfile = &p
		}
	}
	if cmd.Flags().Changed("adaptive") {
		cfg.Adaptive = opts.Adaptive
	}
	if cmd.Flags().Changed("show-headers") {
		cfg.ShowHeaders = opts.ShowHeaders
	}
	if cmd.Flags().Changed("show-title") {
		cfg.ShowTitle = opts.ShowTitle
	}
	if cmd.Flags().Changed("request") {
		cfg.RequestFile = opts.Request
	}
	if cmd.Flags().Changed("user-agent") {
		cfg.UserAgent = opts.UserAgent
	}
	if cmd.Flags().Changed("random-agent") {
		cfg.RandomAgent = opts.RandomAgent
	}
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
//   - stderr is not a terminal (piped, redirected, or CI environment)
func shouldEnableProgress(cfg config.Config, noProgress bool) bool {
	if noProgress {
		return false
	}
	if cfg.Quiet {
		return false
	}
	if !console.IsTerminal(os.Stderr.Fd()) {
		return false
	}
	return true
}
