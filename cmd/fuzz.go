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
	"github.com/spf13/pflag"
	"github.com/unsubble/searchit/internal/adaptive"
	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/console"
	"github.com/unsubble/searchit/internal/diagnostics"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/extensions"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/httpclient"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/output/telemetry"
	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/profile"
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
	Baz             string
	Buzz            string
	Threads         int
	Timeout         int
	Delay           string
	Rate            float64
	Strategy        string
	Output          string
	Format          string
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
	HTTPVersion     string
	Data            string
	Headers         []string
	Cookie          string
	Proxy           string
	Insecure        bool
	Request         string
	UserAgent       string
	Adaptive        bool
	ShowHeaders     bool
	ShowTitle       bool
	HumanReadable   bool
	RandomAgent     bool
	NoProgress      bool
	HelpAll         bool

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

			if opts.Wordlist != "" {
				file, err := os.Open(opts.Wordlist)
				if err != nil {
					return fmt.Errorf("failed to open wordlist:\n\n    %s\n\n%w", opts.Wordlist, err)
				}
				_ = file.Close()
			}

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

			if opts.HTTPVersion != "" {
				if err := httpclient.ValidateHTTPVersion(opts.HTTPVersion); err != nil {
					return err
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
				if strategyLower != "eager" && strategyLower != "bfs" && strategyLower != "dfs" && strategyLower != "priority" {
					return fmt.Errorf("invalid --strategy: %q (must be eager, bfs, dfs, or priority)", opts.Strategy)
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

			// 1. Resolve profiles (left → right).
			var profileOverlays []config.FuzzOverlay
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

						// Decode into config.FuzzOverlay
						var overlay config.FuzzOverlay
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
			cfg, err := config.ResolveFuzzConfig(cfgFile, profileOverlays)
			if err != nil {
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
				fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
				return fmt.Errorf("config error")
			}

			// 3. Apply CLI flag overrides to ensure they take precedence
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

			// Validate and resolve secondary placeholder aliases before loading any wordlist.
			// An alias like "=fuzz" means: reuse the FUZZ wordlist without reopening the file.
			aliasMap := map[string]string{
				"FOO":  opts.Foo,
				"BAR":  opts.Bar,
				"BAZ":  opts.Baz,
				"BUZZ": opts.Buzz,
			}
			if err := validatePlaceholderAliases(aliasMap); err != nil {
				return err
			}

			// Load auxiliary wordlists. Order matters: aliases reference previously-loaded slices.
			// The loaded map is keyed by canonical placeholder name (upper-case).
			loadedWords := map[string][]string{}
			// FUZZ is the primary; it will be loaded later via primaryReader.
			// Pre-populate with an empty sentinel so aliases to FUZZ are valid.
			// The actual slice will be filled in at runtime by the Runner's primary channel.
			// For alias resolution we need the file slice only for secondary placeholders;
			// FUZZ is the only one that may be aliased without having a pre-loaded slice.
			// We handle that case specially: if a secondary aliases =fuzz we will load
			// FUZZ's file upfront so the slice is available.
			var fuzzFileWords []string // non-nil only when a secondary aliases =fuzz

			// Determine if any secondary aliases =fuzz so we can pre-load the FUZZ file.
			for _, raw := range []string{opts.Foo, opts.Bar, opts.Baz, opts.Buzz} {
				if isAlias(raw) && strings.EqualFold(strings.TrimPrefix(raw, "="), "fuzz") {
					// Pre-load the FUZZ file once.
					if fuzzFileWords == nil {
						fuzzFileWords, err = loadLines(opts.Wordlist)
						if err != nil {
							return fmt.Errorf("failed to pre-load FUZZ wordlist for alias: %w", err)
						}
						loadedWords["FUZZ"] = fuzzFileWords
					}
					break
				}
			}

			fooWords, err := resolveSecondaryWordlist(opts.Foo, "FOO", loadedWords)
			if err != nil {
				return fmt.Errorf("failed to load FOO wordlist: %w", err)
			}
			loadedWords["FOO"] = fooWords

			barWords, err := resolveSecondaryWordlist(opts.Bar, "BAR", loadedWords)
			if err != nil {
				return fmt.Errorf("failed to load BAR wordlist: %w", err)
			}
			loadedWords["BAR"] = barWords

			bazWords, err := resolveSecondaryWordlist(opts.Baz, "BAZ", loadedWords)
			if err != nil {
				return fmt.Errorf("failed to load BAZ wordlist: %w", err)
			}
			loadedWords["BAZ"] = bazWords

			buzzWords, err := resolveSecondaryWordlist(opts.Buzz, "BUZZ", loadedWords)
			if err != nil {
				return fmt.Errorf("failed to load BUZZ wordlist: %w", err)
			}
			loadedWords["BUZZ"] = buzzWords

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
				return fmt.Errorf("no placeholders (FUZZ, FOO, BAR, BAZ, BUZZ) found in URL, body, cookies or headers")
			}

			usesFUZZ, usesFOO, usesBAR, usesBAZ, usesBUZZ := false, false, false, false, false
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
				if p == "BAZ" {
					usesBAZ = true
				}
				if p == "BUZZ" {
					usesBUZZ = true
				}
			}

			if usesFOO && opts.Foo == "" {
				return fmt.Errorf("placeholder FOO is used but no --foo wordlist provided")
			}
			if usesBAR && opts.Bar == "" {
				return fmt.Errorf("placeholder BAR is used but no --bar wordlist provided")
			}
			if usesBAZ && opts.Baz == "" {
				return fmt.Errorf("placeholder BAZ is used but no --baz wordlist provided")
			}
			if usesBUZZ && opts.Buzz == "" {
				return fmt.Errorf("placeholder BUZZ is used but no --buzz wordlist provided")
			}

			// Determine output file and output formatters.
			var fileFmttr output.Formatter
			if opts.Output != "" {
				f, err := os.OpenFile(opts.Output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
				if err != nil {
					return fmt.Errorf("cannot open output file: %w", err)
				}
				defer f.Close()

				fileFmt := output.FormatText
				if cmd.Flags().Changed("format") {
					if parsed, err := output.Parse(opts.Format); err == nil {
						fileFmt = parsed
					}
				} else {
					fileFmt = output.FormatFromPath(opts.Output)
				}

				fileQuiet := cfg.Quiet && !cmd.Flags().Changed("format") && fileFmt == output.FormatText
				fileFmttr = output.New(fileFmt, f, fileQuiet, cfg.ShowHeaders, cfg.ShowTitle, cfg.HumanReadableSizes)
				if fileFmttr != nil {
					defer fileFmttr.Close()
				}
			}

			// Terminal formatter setup: always created; -q selects quiet (links-only) format
			var termFmttr output.Formatter
			termFmt := output.FormatText
			if cmd.Flags().Changed("format") {
				if parsed, err := output.Parse(opts.Format); err == nil {
					termFmt = parsed
				}
			}
			termFmttr = output.New(termFmt, cmd.OutOrStdout(), cfg.Quiet, cfg.ShowHeaders, cfg.ShowTitle, cfg.HumanReadableSizes)
			if termFmttr != nil {
				defer termFmttr.Close()
			}

			var baseCount int
			var primaryReader wordlist.Reader
			if opts.Wordlist != "" {
				primaryReader = wordlist.FileReader{Path: opts.Wordlist}
			} else {
				primaryReader = wordlist.EmbeddedReader{}
			}
			if countable, ok := primaryReader.(wordlist.Countable); ok {
				if cnt, err := countable.Count(); err == nil {
					baseCount = cnt
					if len(cfg.Extensions) > 0 {
						baseCount *= (1 + len(cfg.Extensions))
					}
				}
			}

			appState := app.New(ctx, cfg)
			httpclient.ConfigureTransportForWorkers(appState.HTTPClient, cfg.Threads)

			targetManager := targets.NewManager(opts.resolvedFuzzTargets)
			globalSummary := targets.NewGlobalSummary(len(opts.resolvedFuzzTargets))

			errExecute := targetManager.Execute(ctx, func(tCtx targets.TargetContext) error {
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

				fuzzCtx := tCtx.Ctx
				cancelSig := tCtx.Cancel
				targetURL := resolvedURL

				if cfg.Adaptive {
					if appState.AdaptiveEngine == nil || appState.AdaptiveEngine.TargetURL == "" {
						appState.AdaptiveEngine = adaptive.NewEngine(targetURL, appState.HTTPClient, appState.FingerprintCache, cfg.Quiet)
					} else {
						appState.AdaptiveEngine.TargetURL = targetURL
					}
				}

				stateMgr := state.NewManager()
				stateMgr.Transition(state.PhaseStarting)

				// 1. Create a fresh TerminalManager for the fuzz lifecycle (writing to stderr).
				tm := terminal.New(cmd.ErrOrStderr())
				if err := tm.AcquireOwner(terminal.OwnerConfiguration); err != nil {
					return err
				}

				var totalCandidates int64
				if !cfg.Quiet {
					wordlistsCount := 0
					primaryWl := opts.Wordlist
					if primaryWl == "" {
						primaryWl = "embedded"
					}
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
					if usesBAZ {
						wordlistsCount++
						if primaryWl == "" {
							primaryWl = opts.Baz
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
						BazWords:        bazWords,
						BuzzWords:       buzzWords,
					}
					totalCandidates = tmpRunner.EstimateCandidates(baseCount)

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

				// Transition out of Starting, into Running, and hand over to Progress.
				if err := tm.TransitionAndRelease(terminal.PhaseRunning, terminal.OwnerConfiguration); err != nil {
					return err
				}
				if err := tm.AcquireOwner(terminal.OwnerProgress); err != nil {
					return err
				}

				// Setup Context is handled globally above.

				collector := stats.NewCollector()
				if totalCandidates > 0 {
					collector.SetTotalWork(totalCandidates)
				}

				var progMgr *progress.Manager
				var renderer *progress.ANSIRenderer
				var progCmdChan chan console.Command
				var consoleCtrl *console.Controller

				enableProgress := shouldEnableProgress(cfg, opts.NoProgress)
				interactive := enableProgress && console.IsTerminal(os.Stdin.Fd())

				var progDone chan struct{}
				progCtx, cancelProg := context.WithCancel(ctx)
				defer cancelProg()

				termCtx, cancelTerm := context.WithCancel(ctx)
				defer cancelTerm()

				if enableProgress {
					modeStr := fmt.Sprintf("Fuzz (%s)", strings.ToUpper(cfg.FuzzStrategy))
					renderer = progress.NewANSIRenderer(tm, targetURL, nil, modeStr)
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

					// Diagnostic: dump structured state if shutdown takes too long
					diagTimeout := cfg.Timeout + 2*time.Second
					if diagTimeout < 5*time.Second {
						diagTimeout = 5 * time.Second
					}
					go diagnostics.RunDiagnostics(diagTimeout, fuzzCtx.Err(), drainCtx.Err())
				}()

				stateMgr.Transition(state.PhaseRunning)

				var readerWg sync.WaitGroup
				var primaryChan chan string
				if opts.Wordlist != "" || usesFUZZ {
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
					BazWords:        bazWords,
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
					AdaptiveEngine:  appState.AdaptiveEngine,
					Cache:           appState.FingerprintCache,
					PauseBlocker:    stateMgr.WaitUntilRunning,
				}

				estCandidates := runner.EstimateCandidates(baseCount)
				if estCandidates > 0 {
					collector.SetTotalCandidates(estCandidates)
				}

				// Wire adaptive informational messages through the progress renderer
				// so they don't overlap the live UI. PrintAbove writes through the
				// terminal manager's locked writer (w) — the same path used by all
				// other progress output — so no raw os.Stderr bypass occurs.
				// The same handler is shared by both the engine ([INFO] messages)
				// and the runner (priority-score / traversal-decision blocks).
				adaptiveInfoHandler := func(msg string) {
					if progMgr != nil {
						progMgr.PrintAbove(msg)
					} else {
						fmt.Fprintln(os.Stderr, msg)
					}
				}
				runner.InfoHandler = adaptiveInfoHandler
				if cfg.Adaptive && appState.AdaptiveEngine != nil {
					appState.AdaptiveEngine.InfoHandler = adaptiveInfoHandler
				}

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
							FuzzData:    r.FuzzData,
						}
						if termFmttr != nil {
							if progMgr != nil {
								progMgr.ExecuteAbove(func() {
									_ = termFmttr.Print(engRes)
								})
							} else {
								_ = termFmttr.Print(engRes)
							}
						}
						if fileFmttr != nil {
							_ = fileFmttr.Print(engRes)
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
				if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) {
					return runErr
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

			if errExecute != nil && !errors.Is(errExecute, context.Canceled) && !errors.Is(errExecute, context.DeadlineExceeded) {
				return errExecute
			}

			if len(opts.resolvedFuzzTargets) > 1 && !cfg.Quiet {
				fmt.Fprintf(os.Stderr, "\n[*] Global Summary:\n    Targets scanned: %d/%d\n    Total Requests: %d\n    Total Discoveries: %d\n    Duration: %s\n",
					globalSummary.TargetsRun, globalSummary.TargetsTotal, globalSummary.TotalJobs, globalSummary.TotalFound, globalSummary.Duration())
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&opts.URL, "url", "u", "", "target URL with placeholders (FUZZ, FOO, BAR, BAZ, BUZZ)")
	cmd.Flags().StringVar(&opts.URLFile, "url-file", "", "Target URL file (NOT SUPPORTED in fuzz)")
	cmd.Flags().StringVarP(&opts.Wordlist, "wordlist", "w", "", "primary wordlist path (maps to FUZZ)")
	cmd.Flags().StringSliceVarP(&opts.Ext, "ext", "e", nil, "comma-separated extensions or @file")
	cmd.Flags().StringVar(&opts.Foo, "foo", "", "wordlist path for FOO placeholder")
	cmd.Flags().StringVar(&opts.Bar, "bar", "", "wordlist path for BAR placeholder")
	cmd.Flags().StringVar(&opts.Baz, "baz", "", "wordlist path for BAZ placeholder")
	cmd.Flags().StringVar(&opts.Buzz, "buzz", "", "wordlist path for BUZZ placeholder")
	cmd.Flags().StringVarP(&opts.Method, "method", "X", "GET", "HTTP method")
	cmd.Flags().StringVar(&opts.HTTPVersion, "http-version", "auto", "Select the HTTP protocol version (auto, 0.9, 1.0, 1.1, 2)")
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
	cmd.Flags().StringVar(&opts.Delay, "delay", "", "delay between requests (e.g. 50ms, 1s)")
	cmd.Flags().Float64Var(&opts.Rate, "rate", 0, "maximum requests per second rate limit")
	cmd.Flags().BoolVar(&opts.NoProgress, "no-progress", false, "disable progress output")
	cmd.Flags().StringVarP(&opts.Cookie, "cookie", "b", "", "HTTP request cookies with placeholders")
	cmd.Flags().StringVar(&opts.Proxy, "proxy", "", "HTTP proxy URL to use for requests")
	cmd.Flags().BoolVarP(&opts.Insecure, "insecure", "k", false, "skip TLS certificate verification")

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
	cmd.Flags().BoolVarP(&opts.HumanReadable, "human-readable", "R", false, "Render response sizes using human-readable units (KB, MB, GB).")
	cmd.Flags().StringVar(&opts.Request, "request", "", "load raw HTTP request template from file")
	cmd.Flags().StringVarP(&opts.RawProfile, "profile", "p", "", "apply one or more profiles (comma-separated)")
	cmd.Flags().BoolVar(&opts.FollowRedirects, "follow-redirects", false, "follow HTTP redirects")
	cmd.Flags().IntVar(&opts.MaxRedirects, "max-redirects", 10, "maximum redirect limit")
	cmd.Flags().StringVarP(&opts.Strategy, "strategy", "s", "eager", "Traversal strategy (eager, bfs, dfs, priority)")
	cmd.Flags().BoolVar(&opts.Adaptive, "adaptive", false, "enable adaptive fuzzing (prioritization, framework detection, robots.txt, sitemaps)")
	cmd.Flags().StringVar(&opts.UserAgent, "user-agent", "", "set a custom User-Agent for every request")
	cmd.Flags().BoolVar(&opts.RandomAgent, "random-agent", false, "use a randomly selected built-in User-Agent")
	cmd.Flags().BoolVar(&opts.HelpAll, "help-all", false, "show all available options")

	setupCmdHelp(cmd, func() bool { return opts.HelpAll }, fuzzHelpConfig)
	return cmd, opts
}

var fuzzHelpConfig = HelpConfig{
	Examples: `  searchit fuzz -u https://example.com/FUZZ -w words.txt
  searchit fuzz -u https://example.com/FOO/BAR \
    --foo users.txt \
    --bar passwords.txt
  searchit fuzz --request request.txt \
    --foo users.txt`,
	Groups: []FlagGroup{
		{
			Title: "General",
			Names: []string{"url", "wordlist", "foo", "bar", "buzz"},
		},
		{
			Title: "HTTP",
			Names: []string{"method", "cookie", "data", "header"},
		},
		{
			Title: "Discovery",
			Names: []string{"adaptive", "ext", "profile", "strategy"},
		},
		{
			Title: "Matching / Filtering",
			Names: []string{"mc", "ms", "mr", "fc", "fs", "fr"},
		},
		{
			Title: "Output",
			Names: []string{"output", "quiet", "human-readable"},
		},
		{
			Title: "Performance",
			Names: []string{"threads", "random-agent"},
		},
	},
	HelpAllCmd: "searchit fuzz --help-all",
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
	if cmd.Flags().Changed("human-readable") {
		cfg.HumanReadableSizes = opts.HumanReadable
	}
	if cmd.Flags().Changed("request") {
		cfg.RequestFile = opts.Request
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
	if cmd.Flags().Changed("insecure") {
		cfg.Insecure = opts.Insecure
	}
	if cmd.Flags().Changed("wordlist") {
		cfg.Wordlist = opts.Wordlist
	}
	if cmd.Flags().Changed("ext") {
		if exts, err := extensions.Parse(opts.Ext); err == nil {
			cfg.Extensions = exts
		}
	}
	if opts.Strategy != "" {
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
}

// isAlias reports whether a secondary placeholder flag value is an alias
// (i.e. starts with '=' to reference another placeholder's wordlist).
func isAlias(raw string) bool {
	return strings.HasPrefix(raw, "=")
}

// validatePlaceholderAliases checks that alias values (=<name>) are valid:
//   - the referenced name must be a known placeholder (fuzz, foo, bar, baz, buzz)
//   - no self-aliases (e.g. --foo =foo)
//   - no circular chains (e.g. --foo =bar && --bar =foo)
func validatePlaceholderAliases(secondaryMap map[string]string) error {
	known := map[string]bool{"fuzz": true, "foo": true, "bar": true, "baz": true, "buzz": true}

	// Build a directed alias graph: src -> target (both lower-case)
	aliasGraph := map[string]string{}
	for placeholder, raw := range secondaryMap {
		if !isAlias(raw) {
			continue
		}
		src := strings.ToLower(placeholder)
		dst := strings.ToLower(strings.TrimPrefix(raw, "="))
		if !known[dst] {
			return fmt.Errorf("alias %q for --%s references unknown placeholder %q; valid targets: fuzz, foo, bar, baz, buzz",
				raw, strings.ToLower(placeholder), dst)
		}
		if src == dst {
			return fmt.Errorf("alias %q for --%s is a self-alias; a placeholder cannot reference itself",
				raw, strings.ToLower(placeholder))
		}
		aliasGraph[src] = dst
	}

	// Detect circular chains via DFS.
	for start := range aliasGraph {
		visited := map[string]bool{}
		cur := start
		for {
			if visited[cur] {
				return fmt.Errorf("circular alias detected: chain starting at --%s loops back to %q", start, cur)
			}
			visited[cur] = true
			next, ok := aliasGraph[cur]
			if !ok {
				break
			}
			cur = next
		}
	}
	return nil
}

// resolveSecondaryWordlist loads or aliases a secondary placeholder wordlist.
// If raw starts with '=', it is an alias to an already-resolved placeholder's words.
// loaded contains words indexed by upper-case placeholder name (e.g. "FUZZ", "FOO").
func resolveSecondaryWordlist(raw, placeholderName string, loaded map[string][]string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	if isAlias(raw) {
		target := strings.ToUpper(strings.TrimPrefix(raw, "="))
		words, ok := loaded[target]
		if !ok {
			// This should have been caught by validatePlaceholderAliases, but be defensive.
			return nil, fmt.Errorf("--%s alias %q references %q which has no loaded wordlist",
				strings.ToLower(placeholderName), raw, target)
		}
		// Return a copy of the slice so the caller gets an independent
		// view over the same content — the Runner/Generator will
		// iterate it independently for Cartesian products.
		result := make([]string, len(words))
		copy(result, words)
		return result, nil
	}
	return loadLines(raw)
}
