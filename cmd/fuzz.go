package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"regexp"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/console"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/extensions"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/output/telemetry"
	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/profile"
	fuzzProfile "github.com/unsubble/searchit/internal/profile/fuzz"
	"github.com/unsubble/searchit/internal/profile/resolver"
	"github.com/unsubble/searchit/internal/progress"
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

type FuzzOptions struct {
	URL             string
	URLFile         string // registered but not supported in fuzz
	Wordlist        string
	Ext             []string
	Foo             string
	Bar             string
	Buzz            string
	Threads         int
	Timeout         int
	Delay           string
	Rate            float64
	Strategy        string
	Output          string
	Format          string
	LogCount        int
	Profiles        []string
	RawProfile      string
	Quiet           bool
	FollowRedirects bool
	MaxRedirects    int
	ExcludeStatus   string
	MatchStatus     string
	FilterStatus    string
	IncludeSize     string
	ExcludeSize     string
	MatchSize       string
	FilterSize      string
	IncludeHeaders  []string
	ExcludeHeaders  []string
	MatchRegex      []string
	FilterRegex     []string
	MatchContent    []string
	FilterContent   []string
	Method          string
	Data            string
	Headers         []string
	Cookie          string
	Proxy           string
	Request         string
	UserAgent       string
	Adaptive        bool
	ShowHeaders     bool
	ShowTitle       bool
	RandomAgent     bool
	NoProgress      bool

	resolvedFuzzTargets   []targets.Target
	testHookConfigApplied func(config.Config)
}

type structuralErr string

func (e structuralErr) Error() string { return string(e) }

func fuzzZeroTargetError() error {
	return structuralErr(`
error:

    fuzz requires exactly one target.

    Supported:

        searchit fuzz -u URL

    or

        searchit fuzz --request request.txt
`)
}

func fuzzTargetError() error {
	return structuralErr(`
error:

    fuzz accepts exactly one target.

    Multiple target execution is
    intentionally not supported by
    the fuzz subsystem.

    Supported:

        searchit fuzz -u URL

    or

        searchit fuzz --request request.txt

    For multiple targets use:

        searchit scan --url-file ...

    or execute fuzz separately
    for each target.
`)
}

func NewFuzzCmd() (*cobra.Command, *FuzzOptions) {
	opts := &FuzzOptions{}
	cmd := &cobra.Command{
		Use:   "fuzz",
		Short: "Fuzz parameters, paths, subdomains and bodies",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.RawProfile != "" {
				for _, p := range strings.Split(opts.RawProfile, ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						opts.Profiles = append(opts.Profiles, p)
					}
				}
			}

			hasURL := opts.URL != "" || opts.Request != ""
			hasProfiles := len(opts.Profiles) > 0

			if !hasURL && !hasProfiles {
				return fuzzZeroTargetError()
			}
			if opts.URL != "" && opts.Request != "" {
				return fuzzTargetError()
			}
			if opts.URLFile != "" {
				return fuzzTargetError()
			}
			if opts.Threads < 1 {
				return fmt.Errorf("threads must be at least 1")
			}

			if hasURL {
				var errParse error
				opts.resolvedFuzzTargets, errParse = targets.Parse(targets.ParseOptions{
					URL:         opts.URL,
					URLFile:     opts.URLFile,
					RequestFile: opts.Request,
				})
				if errParse != nil {
					return fuzzTargetError()
				}
				if len(opts.resolvedFuzzTargets) != 1 {
					return fuzzTargetError()
				}
			}
			// else: profiles provided with no URL — defer target validation to RunE.

			if opts.ExcludeStatus != "" {
				if _, err := status.Parse(opts.ExcludeStatus); err != nil {
					return fmt.Errorf("invalid exclude-status: %w", err)
				}
			}

			if opts.IncludeSize != "" {
				if _, err := size.Parse(opts.IncludeSize); err != nil {
					return fmt.Errorf("invalid include-size: %w", err)
				}
			}
			if opts.ExcludeSize != "" {
				if _, err := size.Parse(opts.ExcludeSize); err != nil {
					return fmt.Errorf("invalid exclude-size: %w", err)
				}
			}

			if opts.Delay != "" {
				if _, err := time.ParseDuration(opts.Delay); err != nil {
					return fmt.Errorf("invalid delay: %w", err)
				}
			}

			if cmd.Flags().Changed("rate") && opts.Rate <= 0 {
				return fmt.Errorf("rate must be greater than 0")
			}

			if opts.Output != "" {
				if fi, err := os.Stat(opts.Output); err == nil && fi.IsDir() {
					return fmt.Errorf("--output %q is a directory; provide a file path", opts.Output)
				}
			}

			if cmd.Flags().Changed("format") {
				if _, err := output.Parse(opts.Format); err != nil {
					return fmt.Errorf("invalid --format: %w", err)
				}
			}

			if opts.Proxy != "" {
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

			if opts.MaxRedirects < 0 {
				return fmt.Errorf("max-redirects cannot be negative")
			}

			if opts.Strategy != "" {
				strategyLower := strings.ToLower(opts.Strategy)
				if strategyLower != "eager" && strategyLower != "bfs" && strategyLower != "dfs" {
					return fmt.Errorf("invalid --strategy: %q (must be eager, bfs, or dfs)", opts.Strategy)
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			stats.GlobalInstrumentation.Reset()
			atomic.StoreInt32(&stats.GlobalInstrumentation.Enabled, 1)

			ctx := cmd.Context()
			ctx, cancelGraceful := context.WithCancel(ctx)
			defer cancelGraceful()

			var activeTargetCtx context.Context
			var activeTargetMu sync.Mutex

			drainCtx, cancelDrain := context.WithCancel(context.Background())
			defer cancelDrain()

			signals.SetupGlobal(drainCtx, func() {
				if ctx.Err() != nil {
					cancelDrain()
					return
				}
				activeTargetMu.Lock()
				tgtCtx := activeTargetCtx
				activeTargetMu.Unlock()
				if tgtCtx != nil && tgtCtx.Err() != nil {
					// The target is already draining. Force abort.
					cancelDrain()
					return
				}
				// graceful
				cancelGraceful()
			}, func() {
				// force abort
				cancelDrain()
			})

			// Initialize default config and HTTP client options
			cfg := config.Default()
			cfg.Threads = opts.Threads
			cfg.Timeout = time.Duration(opts.Timeout) * time.Second
			cfg.Quiet = opts.Quiet
			cfg.Proxy = opts.Proxy
			cfg.ShowHeaders = opts.ShowHeaders
			cfg.ShowTitle = opts.ShowTitle

			if opts.MatchStatus != "" {
				exclude, _ := status.Parse(opts.MatchStatus)
				cfg.Status.Include = exclude
			}
			if opts.FilterStatus != "" {
				exclude, _ := status.Parse(opts.FilterStatus)
				cfg.Status.Exclude = exclude
			} else if opts.ExcludeStatus != "" {
				exclude, _ := status.Parse(opts.ExcludeStatus)
				cfg.Status.Exclude = exclude
			}

			if opts.MatchSize != "" {
				inc, _ := size.Parse(opts.MatchSize)
				cfg.IncludeSize = inc
			} else if opts.IncludeSize != "" {
				inc, _ := size.Parse(opts.IncludeSize)
				cfg.IncludeSize = inc
			}
			if opts.FilterSize != "" {
				exc, _ := size.Parse(opts.FilterSize)
				cfg.ExcludeSize = exc
			} else if opts.ExcludeSize != "" {
				exc, _ := size.Parse(opts.ExcludeSize)
				cfg.ExcludeSize = exc
			}
			cfg.MatchRegex = opts.MatchRegex
			cfg.FilterRegex = opts.FilterRegex
			cfg.MatchContent = opts.MatchContent
			cfg.FilterContent = opts.FilterContent

			// Apply profiles if specified
			if len(opts.Profiles) > 0 {
				store := profile.NewStore()

				for _, profileName := range opts.Profiles {
					resolved, err := resolver.New(store).Resolve([]string{profileName})
					if err != nil {
						cmd.SilenceErrors = true
						cmd.SilenceUsage = true
						fmt.Fprintf(cmd.ErrOrStderr(), "failed to load profile:\n%v\n", err)
						return fmt.Errorf("load failed")
					}

					for _, p := range resolved {
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

						// Decode and Apply
						var overlay fuzzProfile.Overlay
						if err := p.Decode(&overlay); err != nil {
							cmd.SilenceErrors = true
							cmd.SilenceUsage = true
							fmt.Fprintf(cmd.ErrOrStderr(), "failed to load profile:\n%v\n", err)
							return fmt.Errorf("decode failed")
						}
						fuzzProfile.Apply(&cfg, overlay)
					}
				}
			}

			// Apply CLI overrides to ensure they take precedence
			applyFuzzCLIOverrides(opts, cmd, &cfg)

			// If no CLI URL was given but a profile set one, propagate it now.
			if opts.URL == "" && len(cfg.URLs) > 0 {
				opts.URL = cfg.URLs[0]
			}
			if opts.URL == "" && opts.Request == "" {
				return fuzzZeroTargetError()
			}

			if opts.testHookConfigApplied != nil {
				opts.testHookConfigApplied(cfg)
			}

			// Propagate resolved values to command execution
			opts.Method = cfg.Method
			opts.Data = cfg.Data
			opts.Cookie = cfg.Cookies
			opts.Request = cfg.RequestFile

			var delay time.Duration
			if cfg.Delay > 0 {
				delay = cfg.Delay
			}

			var limiter *rate.Limiter
			if cfg.Rate > 0 {
				limiter = rate.NewLimiter(rate.Limit(cfg.Rate), 1)
			}

			// Load auxiliary wordlists
			fooWords, err := loadLines(opts.Foo)
			if err != nil {
				return fmt.Errorf("failed to load FOO wordlist: %w", err)
			}
			barWords, err := loadLines(opts.Bar)
			if err != nil {
				return fmt.Errorf("failed to load BAR wordlist: %w", err)
			}
			buzzWords, err := loadLines(opts.Buzz)
			if err != nil {
				return fmt.Errorf("failed to load BUZZ wordlist: %w", err)
			}

			var headers http.Header
			if opts.Request != "" && len(opts.resolvedFuzzTargets) > 0 {
				t := opts.resolvedFuzzTargets[0]
				opts.URL = t.URL
				opts.Method = t.Method
				opts.Data = t.Body

				headers = make(http.Header)
				for _, h := range t.Headers {
					idx := strings.Index(h, ":")
					if idx != -1 {
						headers.Add(strings.TrimSpace(h[:idx]), strings.TrimSpace(h[idx+1:]))
					}
				}

				cliHeaders, err := parseFuzzHeaderFlags(cfg.Headers)
				if err != nil {
					return err
				}
				for k, values := range cliHeaders {
					for _, v := range values {
						headers.Add(k, v)
					}
				}

				if t.Cookies != "" {
					opts.Cookie = t.Cookies
				}
			} else {
				cliHeaders, err := parseFuzzHeaderFlags(cfg.Headers)
				if err != nil {
					return err
				}
				headers = cliHeaders
			}

			// Resolve and inject User-Agent once, before the runner is created.
			// Resolution order: -H "User-Agent=..." > --user-agent > profile/--random-agent.
			var randomUA string
			if cfg.RandomAgent {
				randomUA = useragent.Random()
			}
			if ua := useragent.Resolve(headers.Get("User-Agent"), cfg.UserAgent, randomUA); ua != "" {
				if headers == nil {
					headers = make(http.Header)
				}
				headers.Set("User-Agent", ua)
			}

			reqTmpl := fuzz.RequestTemplate{
				URL:     opts.URL,
				Method:  opts.Method,
				Body:    opts.Data,
				Headers: headers,
				Cookie:  opts.Cookie,
			}
			detectedPlaceholders := fuzz.FindPlaceholders(reqTmpl)

			if len(detectedPlaceholders) == 0 {
				return fmt.Errorf("no placeholders (FUZZ, FOO, BAR, BUZZ) found in URL, body, cookies or headers")
			}

			usesFUZZ, usesFOO, usesBAR, usesBUZZ := false, false, false, false
			for _, p := range detectedPlaceholders {
				if p == "FUZZ" {
					usesFUZZ = true
				}
				if p == "FOO" {
					usesFOO = true
				}
				if p == "BAR" {
					usesBAR = true
				}
				if p == "BUZZ" {
					usesBUZZ = true
				}
			}

			if usesFUZZ && opts.Wordlist == "" {
				return fmt.Errorf("placeholder FUZZ is used but no primary wordlist provided (use -w or --wordlist)")
			}
			if usesFOO && opts.Foo == "" {
				return fmt.Errorf("placeholder FOO is used but no --foo wordlist provided")
			}
			if usesBAR && opts.Bar == "" {
				return fmt.Errorf("placeholder BAR is used but no --bar wordlist provided")
			}
			if usesBUZZ && opts.Buzz == "" {
				return fmt.Errorf("placeholder BUZZ is used but no --buzz wordlist provided")
			}

			// Resolve output format
			outFormat := output.FormatText
			if opts.Output != "" {
				outFormat = output.FormatFromPath(opts.Output)
			}
			if cmd.Flags().Changed("format") {
				parsedFormat, _ := output.Parse(opts.Format)
				outFormat = parsedFormat
			}

			var outWriter io.Writer = os.Stdout
			if opts.Output != "" {
				f, err := os.OpenFile(opts.Output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
				if err != nil {
					return fmt.Errorf("cannot open output file: %w", err)
				}
				defer f.Close()
				outWriter = f
			}

			if outFormat != output.FormatText && outWriter == os.Stdout {
				cfg.Quiet = true
			}

			baseCount := 4600 // Embedded list size
			if opts.Wordlist != "" {
				baseCount = countFileLines(opts.Wordlist)
			}
			if len(cfg.Extensions) > 0 {
				baseCount *= (1 + len(cfg.Extensions))
			}
			targetManager := targets.NewManager(opts.resolvedFuzzTargets)
			globalSummary := targets.NewGlobalSummary(len(opts.resolvedFuzzTargets))

			errExecute := targetManager.Execute(ctx, func(tCtx targets.TargetContext) error {
				activeTargetMu.Lock()
				activeTargetCtx = tCtx.Ctx
				activeTargetMu.Unlock()

				fuzzCtx := tCtx.Ctx
				cancelSig := tCtx.Cancel
				targetURL := tCtx.Target.URL

				stateMgr := state.NewManager()
				stateMgr.Transition(state.PhaseStarting)

				// 1. Create a fresh TerminalManager for the fuzz lifecycle.
				tm := terminal.New(os.Stdout)
				if err := tm.AcquireOwner(terminal.OwnerConfiguration); err != nil {
					return err
				}

				// 2. Formatters & Telemetry output setup.
				var fmttr output.Formatter
				if outWriter == os.Stdout {
					fmttr = output.NewWithManager(outFormat, tm, terminal.OwnerProgress, cfg.Quiet, cfg.ShowHeaders, cfg.ShowTitle)
				} else {
					fmttr = output.New(outFormat, outWriter, cfg.Quiet, cfg.ShowHeaders, cfg.ShowTitle)
				}

				if !cfg.Quiet {
					wordlistsCount := 0
					primaryWl := opts.Wordlist
					if usesFUZZ {
						wordlistsCount++
					}
					if usesFOO {
						wordlistsCount++
						if primaryWl == "" {
							primaryWl = opts.Foo
						}
					}
					if usesBAR {
						wordlistsCount++
						if primaryWl == "" {
							primaryWl = opts.Bar
						}
					}
					if usesBUZZ {
						wordlistsCount++
						if primaryWl == "" {
							primaryWl = opts.Buzz
						}
					}
					placeholdersStr := fmt.Sprintf("%s (%d)", strings.Join(detectedPlaceholders, ", "), len(detectedPlaceholders))

					excludeStatusStr := cfg.Status.Exclude.String()
					if excludeStatusStr == "" {
						excludeStatusStr = "none"
					}

					tmpRunner := &fuzz.Runner{
						TargetURL:       targetURL,
						BodyTemplate:    opts.Data,
						HeaderTemplates: headers,
						CookieTemplate:  opts.Cookie,
						FooWords:        fooWords,
						BarWords:        barWords,
						BuzzWords:       buzzWords,
					}
					totalCandidates := tmpRunner.EstimateCandidates(baseCount)

					if cfg.LogCount > 0 {
						info := telemetry.ConfigInfo{
							Target:          targetURL,
							Method:          cfg.Method,
							Workers:         cfg.Threads,
							Mode:            "Fuzz",
							Traversal:       strings.ToUpper(cfg.FuzzStrategy),
							AdaptiveEnabled: cfg.Adaptive,
							WordlistsCount:  wordlistsCount,
							PrimaryWordlist: primaryWl,
							Placeholders:    placeholdersStr,
							HTTPVersion:     "auto",
							FollowRedirects: cfg.FollowRedirects,
							FilterStatus:    excludeStatusStr,
							TotalCandidates: int(totalCandidates),
							IsFuzz:          true,
							Extensions:      cfg.Extensions,
						}
						telemetry.PrintConfiguration(tm, terminal.OwnerConfiguration, info)
					}
				}

				// Transition out of Starting, into Running, and hand over to Progress.
				if err := tm.TransitionAndRelease(terminal.PhaseRunning, terminal.OwnerConfiguration); err != nil {
					return err
				}
				if err := tm.AcquireOwner(terminal.OwnerProgress); err != nil {
					return err
				}

				// Setup Context is handled globally above.

				collector := stats.NewCollector()

				var progMgr *progress.Manager
				var renderer *progress.ANSIRenderer
				var progCmdChan chan console.Command
				var consoleCtrl *console.Controller

				enableProgress := shouldEnableProgress(cfg, opts.NoProgress || cfg.LogCount == 0)
				interactive := enableProgress && console.IsTerminal(os.Stdin.Fd())

				var progDone chan struct{}
				progCtx, cancelProg := context.WithCancel(ctx)
				defer cancelProg()

				termCtx, cancelTerm := context.WithCancel(ctx)
				defer cancelTerm()

				if enableProgress {
					modeStr := fmt.Sprintf("Fuzz (%s)", strings.ToUpper(cfg.FuzzStrategy))
					renderer = progress.NewANSIRenderer(tm, targetURL, nil, modeStr, cfg.LogCount)
					renderer.IsPaused = func() bool {
						return stateMgr != nil && stateMgr.Current() == state.PhasePaused
					}
					progMgr = progress.NewManager(tm, collector, renderer, 1*time.Second)
					progMgr.ConfiguredThreads = cfg.Threads
					if outWriter != os.Stdout {
						progMgr.Formatter = fmttr
					}

					if interactive {
						consoleCtrl = console.NewController(os.Stdin)
						progCmdChan = make(chan console.Command, 10)
						go consoleCtrl.Start(termCtx)

						go func() {
							for c := range consoleCtrl.Commands() {
								switch c {
								case console.CommandProgress, console.CommandStats:
									select {
									case progCmdChan <- c:
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
									cancelSig()
								case console.CommandAbortAll:
									if ctx.Err() != nil {
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
					<-fuzzCtx.Done() // Triggered when target completes or target context is cancelled

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
				}()

				stateMgr.Transition(state.PhaseRunning)

				var readerWg sync.WaitGroup
				var primaryChan chan string
				if opts.Wordlist != "" || strings.Contains(targetURL, "FUZZ") {
					var reader wordlist.Reader
					if opts.Wordlist != "" {
						reader = wordlist.FileReader{Path: opts.Wordlist}
					} else {
						reader = wordlist.EmbeddedReader{}
					}
					primaryChan = make(chan string, 100)
					readerWg.Add(1)
					go func() {
						defer readerWg.Done()
						defer close(primaryChan)
						tempChan := make(chan string, 100)
						go func() {
							defer close(tempChan)
							_ = reader.Read(ctx, tempChan)
						}()
						for w := range tempChan {
							atomic.AddInt64(&stats.GlobalInstrumentation.WordsRead, 1)
							variants := extensions.GenerateVariants(w, cfg.Extensions)
							for _, v := range variants {
								select {
								case <-ctx.Done():
									return
								case primaryChan <- v:
								}
							}
						}
					}()
				}

				appState := app.New(fuzzCtx, cfg)

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

				runner := &fuzz.Runner{
					TargetURL:       targetURL,
					Method:          strings.ToUpper(opts.Method),
					BodyTemplate:    opts.Data,
					HeaderTemplates: headers,
					CookieTemplate:  opts.Cookie,
					FooWords:        fooWords,
					BarWords:        barWords,
					BuzzWords:       buzzWords,
					Client:          appState.HTTPClient,
					FS:              fs,
					Threads:         cfg.Threads,
					Delay:           delay,
					Limiter:         limiter,
					Collector:       collector,
					Quiet:           cfg.Quiet,
					ShowHeaders:     cfg.ShowHeaders,
					ShowTitle:       cfg.ShowTitle,
					Adaptive:        cfg.Adaptive,
					Cache:           appState.FingerprintCache,
					PauseBlocker:    stateMgr.WaitUntilRunning,
				}

				collector.SetTotalCandidates(runner.EstimateCandidates(baseCount))

				runErr := runner.Run(fuzzCtx, drainCtx, cfg.FuzzStrategy, primaryChan, func(r fuzz.Result) {
					if r.Accepted {
						engRes := engine.Result{
							URL:         r.URL,
							RedirectURL: r.RedirectURL,
							StatusCode:  r.StatusCode,
							Length:      r.Length,
							Accepted:    r.Accepted,
							Err:         r.Err,
							Origin:      "fuzz",
							Title:       r.Title,
							Headers:     r.Headers,
						}
						if progMgr != nil {
							progMgr.HandleResult(engRes)
						} else {
							_ = fmttr.Print(engRes)
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
				if runErr != nil {
					return runErr
				}

				// Fuzzing has naturally completed, synchronize target candidate count
				// with actual JobsProduced in case of BFS/DFS pruning.
				collector.SetTotalCandidates(collector.Snapshot().JobsProduced)

				if fmttr != nil {
					_ = fmttr.Close()
				}

				// In case the target naturally completed, trigger shutdown done.
				cancelSig()
				<-shutdownDone

				stateMgr.Transition(state.PhaseWaitingWorkers)
				_ = tm.TransitionTo(terminal.PhaseWaitingWorkers)

				stateMgr.Transition(state.PhaseFinalizing)
				_ = tm.TransitionTo(terminal.PhaseFinalizing)

				stateMgr.Transition(state.PhaseTerminalShutdown)
				_ = tm.TransitionTo(terminal.PhaseTerminalShutdown)

				if enableProgress && renderer != nil {
					_ = renderer.Close(terminal.OwnerProgress)
				}
				_ = tm.ReleaseOwner(terminal.OwnerProgress)

				stateMgr.Transition(state.PhaseSummary)
				_ = tm.AcquireAndTransition(terminal.OwnerSummary, terminal.PhaseSummary)

				if !cfg.Quiet {
					snap := runner.Collector.Snapshot()

					telemetry.PrintSummary(tm, terminal.OwnerSummary, telemetry.SummaryInfo{
						IsFuzz:          true,
						Mode:            "Fuzz",
						Traversal:       strings.ToUpper(cfg.FuzzStrategy),
						AdaptiveEnabled: cfg.Adaptive,
						Findings:        int(snap.Discovered),
						Snapshot:        snap,
					}, flagDebug)
					_ = tm.TransitionAndRelease(terminal.PhasePipeline, terminal.OwnerSummary)

					stateMgr.Transition(state.PhasePipeline)
					_ = tm.AcquireOwner(terminal.OwnerPipeline)

					if flagDebug {
						if cfg.Adaptive && runner.Summary != nil {
							techs := append([]string(nil), runner.Summary.Technologies...)
							sort.Strings(techs)

							telemetry.PrintAdaptive(tm, terminal.OwnerPipeline, telemetry.AdaptiveInfo{
								Technologies:        techs,
								Discoveries:         runner.Summary.Discoveries,
								DFSCount:            int(runner.Summary.DFSCount),
								BFSCount:            int(runner.Summary.BFSCount),
								EagerCount:          int(runner.Summary.EagerCount),
								HighPriorityCount:   int(runner.Summary.HighPriorityCount),
								MediumPriorityCount: int(runner.Summary.MediumPriorityCount),
								LowPriorityCount:    int(runner.Summary.LowPriorityCount),
							})
						}

						telemetry.PrintPipelineReconciliation(tm, terminal.OwnerPipeline)
					}
					_ = tm.TransitionAndRelease(terminal.PhaseDone, terminal.OwnerPipeline)
				} else {
					_ = tm.TransitionTo(terminal.PhasePipeline)
					stateMgr.Transition(state.PhasePipeline)
					_ = tm.TransitionTo(terminal.PhaseDone)
				}

				stateMgr.Transition(state.PhaseDone)

				globalSummary.AddSnapshot(runner.Collector.Snapshot())

				return nil
			})

			if errExecute != nil && !errors.Is(errExecute, context.Canceled) {
				return errExecute
			}

			if len(opts.resolvedFuzzTargets) > 1 && !cfg.Quiet {
				fmt.Printf("\n[*] Global Summary:\n    Targets scanned: %d/%d\n    Total Requests: %d\n    Total Discoveries: %d\n    Duration: %s\n",
					globalSummary.TargetsRun, globalSummary.TargetsTotal, globalSummary.TotalJobs, globalSummary.TotalFound, globalSummary.Duration())
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&opts.URL, "url", "u", "", "target URL with placeholders (FUZZ, FOO, BAR, BUZZ)")
	cmd.Flags().StringVar(&opts.URLFile, "url-file", "", "Target URL file (NOT SUPPORTED in fuzz)")
	cmd.Flags().StringVarP(&opts.Wordlist, "wordlist", "w", "", "primary wordlist path (maps to FUZZ)")
	cmd.Flags().StringSliceVarP(&opts.Ext, "ext", "e", nil, "comma-separated extensions or @file")
	cmd.Flags().StringVar(&opts.Foo, "foo", "", "wordlist path for FOO placeholder")
	cmd.Flags().StringVar(&opts.Bar, "bar", "", "wordlist path for BAR placeholder")
	cmd.Flags().StringVar(&opts.Buzz, "buzz", "", "wordlist path for BUZZ placeholder")
	cmd.Flags().StringVarP(&opts.Method, "method", "X", "GET", "HTTP method")
	cmd.Flags().StringVarP(&opts.Data, "data", "d", "", "POST request data body with placeholders")
	cmd.Flags().StringSliceVarP(&opts.Headers, "header", "H", nil, "custom request headers with placeholders (e.g. -H 'X-Header=FUZZ')")
	cmd.Flags().IntVarP(&opts.Threads, "threads", "t", 32, "number of concurrent worker threads")
	cmd.Flags().IntVar(&opts.Timeout, "timeout", 10, "request timeout in seconds")
	cmd.Flags().StringVarP(&opts.ExcludeStatus, "exclude-status", "x", "404", "comma-separated status codes to exclude")
	cmd.Flags().StringVar(&opts.IncludeSize, "include-size", "", "comma-separated exact sizes or ranges to include")
	cmd.Flags().StringVar(&opts.ExcludeSize, "exclude-size", "", "comma-separated exact sizes or ranges to exclude")
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "write results to this file (default: stdout)")
	cmd.Flags().StringVar(&opts.Format, "format", "text", "explicit output format (text, json, ndjson, csv, markdown)")
	cmd.Flags().BoolVarP(&opts.Quiet, "quiet", "q", false, "disable status prefix printing in stdout")
	cmd.Flags().IntVar(&opts.LogCount, "log-count", 10, "number of discovery lines visible in the interactive discovery region")
	cmd.Flags().StringVar(&opts.Delay, "delay", "", "delay between requests (e.g. 50ms, 1s)")
	cmd.Flags().Float64Var(&opts.Rate, "rate", 0, "maximum requests per second rate limit")
	cmd.Flags().BoolVar(&opts.NoProgress, "no-progress", false, "disable progress output")
	cmd.Flags().StringVarP(&opts.Cookie, "cookie", "b", "", "HTTP request cookies with placeholders")
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
	cmd.Flags().StringVarP(&opts.RawProfile, "profile", "p", "", "apply one or more profiles (comma-separated)")
	cmd.Flags().BoolVar(&opts.FollowRedirects, "follow-redirects", false, "follow HTTP redirects")
	cmd.Flags().IntVar(&opts.MaxRedirects, "max-redirects", 10, "maximum redirect limit")
	cmd.Flags().StringVarP(&opts.Strategy, "strategy", "s", "eager", "Traversal strategy (eager, bfs, dfs)")
	cmd.Flags().BoolVar(&opts.Adaptive, "adaptive", false, "enable adaptive fuzzing (prioritization, framework detection, robots.txt, sitemaps)")
	cmd.Flags().StringVar(&opts.UserAgent, "user-agent", "", "set a custom User-Agent for every request")
	cmd.Flags().BoolVar(&opts.RandomAgent, "random-agent", false, "use a randomly selected built-in User-Agent")
	return cmd, opts
}

func loadLines(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines, scanner.Err()
}

func parseFuzzHeaderFlags(flags []string) (http.Header, error) {
	headers := make(http.Header)
	for _, h := range flags {
		idx := strings.Index(h, ":")
		if idx == -1 {
			idx = strings.Index(h, "=")
		}
		if idx <= 0 || idx == len(h)-1 {
			return nil, fmt.Errorf("header flag %q must be in Name: Value or Name=Value format", h)
		}
		name := strings.TrimSpace(h[:idx])
		val := strings.TrimSpace(h[idx+1:])
		headers.Add(name, val)
	}
	return headers, nil
}

func applyFuzzCLIOverrides(opts *FuzzOptions, cmd *cobra.Command, cfg *config.Config) {
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
	if cmd.Flags().Changed("quiet") {
		cfg.Quiet = opts.Quiet
	}
	if cmd.Flags().Changed("proxy") {
		cfg.Proxy = opts.Proxy
	}
	if cmd.Flags().Changed("header") {
		cfg.Headers = opts.Headers
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
	if cmd.Flags().Changed("method") {
		cfg.Method = opts.Method
	}
	if cmd.Flags().Changed("data") {
		cfg.Data = opts.Data
	}
	if cmd.Flags().Changed("cookie") {
		cfg.Cookies = opts.Cookie
	}

	if cmd.Flags().Changed("mc") {
		inc, _ := status.Parse(opts.MatchStatus)
		cfg.Status.Include = inc
	}
	if cmd.Flags().Changed("fc") {
		exc, _ := status.Parse(opts.FilterStatus)
		cfg.Status.Exclude = exc
	} else if cmd.Flags().Changed("exclude-status") {
		exc, _ := status.Parse(opts.ExcludeStatus)
		cfg.Status.Exclude = exc
	}

	if cmd.Flags().Changed("ms") {
		inc, _ := size.Parse(opts.MatchSize)
		cfg.IncludeSize = inc
	} else if cmd.Flags().Changed("include-size") {
		inc, _ := size.Parse(opts.IncludeSize)
		cfg.IncludeSize = inc
	}
	if cmd.Flags().Changed("fs") {
		exc, _ := size.Parse(opts.FilterSize)
		cfg.ExcludeSize = exc
	} else if cmd.Flags().Changed("exclude-size") {
		exc, _ := size.Parse(opts.ExcludeSize)
		cfg.ExcludeSize = exc
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
	if cmd.Flags().Changed("follow-redirects") {
		cfg.FollowRedirects = opts.FollowRedirects
	}
	if cmd.Flags().Changed("max-redirects") {
		cfg.MaxRedirects = opts.MaxRedirects
	}
	if cmd.Flags().Changed("wordlist") {
		cfg.Wordlist = opts.Wordlist
	}
	if cmd.Flags().Changed("ext") {
		if exts, err := extensions.Parse(opts.Ext); err == nil {
			cfg.Extensions = exts
		}
	}
	if cmd.Flags().Changed("strategy") {
		cfg.FuzzStrategy = opts.Strategy
	}
	if cmd.Flags().Changed("adaptive") {
		cfg.Adaptive = opts.Adaptive
	}
	if cmd.Flags().Changed("user-agent") {
		cfg.UserAgent = opts.UserAgent
	}
	if cmd.Flags().Changed("random-agent") {
		cfg.RandomAgent = opts.RandomAgent
	}
	if cmd.Flags().Changed("log-count") {
		cfg.LogCount = opts.LogCount
	}
}

func countFileLines(path string) int {
	if path == "" {
		return 1
	}
	f, err := os.Open(path)
	if err != nil {
		return 1
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		count++
	}

	// check for errors encountered by scanner
	if err := scanner.Err(); err != nil {
		return 1
	}
	if count == 0 {
		return 1
	}
	return count
}
