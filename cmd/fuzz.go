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
	"github.com/unsubble/searchit/internal/profile/resolver"
	scanProfile "github.com/unsubble/searchit/internal/profile/scan"
	"github.com/unsubble/searchit/internal/progress"
	"github.com/unsubble/searchit/internal/requesttemplate"
	"github.com/unsubble/searchit/internal/signals"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/state"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/targets"
	"github.com/unsubble/searchit/internal/wordlist"
	"golang.org/x/time/rate"
)

var (
	flagFuzzURL         string
	flagFuzzURLFile     string
	flagFuzzWordlist    string
	flagFuzzExt         []string
	flagFuzzFoo         string
	flagFuzzBar         string
	flagFuzzBuzz        string
	flagFuzzMethod      string
	flagFuzzData        string
	flagFuzzHeaders     []string
	flagFuzzThreads     int
	flagFuzzTimeout     int
	flagFuzzExcludeStat string
	flagFuzzIncSize     string
	flagFuzzExcSize     string
	flagFuzzOutput      string
	flagFuzzFormat      string
	flagFuzzQuiet       bool
	flagFuzzDelay       string
	flagFuzzRate        float64
	flagFuzzNoProgress  bool
	flagFuzzCookie      string
	flagFuzzProxy       string
	flagFuzzStrategy    string
	flagFuzzAdaptive    bool

	flagFuzzMatchStatus     string
	flagFuzzFilterStatus    string
	flagFuzzMatchSize       string
	flagFuzzFilterSize      string
	flagFuzzMatchRegex      []string
	flagFuzzFilterRegex     []string
	flagFuzzMatchContent    []string
	flagFuzzFilterContent   []string
	flagFuzzShowHeaders     bool
	flagFuzzShowTitle       bool
	flagFuzzRequestFile     string
	flagFuzzProfiles        []string
	flagFuzzFollowRedirects bool
	flagFuzzMaxRedirects    int

	resolvedFuzzTargets []targets.Target
)

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

var fuzzCmd = &cobra.Command{
	Use:   "fuzz",
	Short: "Fuzz parameters, paths, subdomains and bodies",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if flagFuzzURL == "" && flagFuzzRequestFile == "" {
			return fuzzZeroTargetError()
		}
		if flagFuzzURL != "" && flagFuzzRequestFile != "" {
			return fuzzTargetError()
		}
		if flagFuzzURLFile != "" {
			return fuzzTargetError()
		}
		if flagFuzzThreads < 1 {
			return fmt.Errorf("threads must be at least 1")
		}

		var errParse error
		resolvedFuzzTargets, errParse = targets.Parse(targets.ParseOptions{
			URL:         flagFuzzURL,
			URLFile:     flagFuzzURLFile,
			RequestFile: flagFuzzRequestFile,
		})
		if errParse != nil {
			return fuzzTargetError()
		}

		if len(resolvedFuzzTargets) != 1 {
			return fuzzTargetError()
		}

		usesFUZZ, usesFOO, usesBAR, usesBUZZ := false, false, false, false
		t := resolvedFuzzTargets[0]
		if strings.Contains(t.URL, "FUZZ") || strings.Contains(t.Body, "FUZZ") {
			usesFUZZ = true
		}
		if strings.Contains(t.URL, "FOO") || strings.Contains(t.Body, "FOO") {
			usesFOO = true
		}
		if strings.Contains(t.URL, "BAR") || strings.Contains(t.Body, "BAR") {
			usesBAR = true
		}
		if strings.Contains(t.URL, "BUZZ") || strings.Contains(t.Body, "BUZZ") {
			usesBUZZ = true
		}

		for _, h := range t.Headers {
			if strings.Contains(h, "FUZZ") {
				usesFUZZ = true
			}
			if strings.Contains(h, "FOO") {
				usesFOO = true
			}
			if strings.Contains(h, "BAR") {
				usesBAR = true
			}
			if strings.Contains(h, "BUZZ") {
				usesBUZZ = true
			}
		}

		for _, h := range flagFuzzHeaders {
			if strings.Contains(h, "FUZZ") {
				usesFUZZ = true
			}
			if strings.Contains(h, "FOO") {
				usesFOO = true
			}
			if strings.Contains(h, "BAR") {
				usesBAR = true
			}
			if strings.Contains(h, "BUZZ") {
				usesBUZZ = true
			}
		}

		if !usesFUZZ && !usesFOO && !usesBAR && !usesBUZZ {
			return fmt.Errorf("no placeholders (FUZZ, FOO, BAR, BUZZ) found in URL, body or headers")
		}

		if usesFUZZ {
			if flagFuzzWordlist == "" {
				return fmt.Errorf("placeholder FUZZ is used but no primary wordlist provided (use -w or --wordlist)")
			}
		}

		if usesFOO {
			if flagFuzzFoo == "" {
				return fmt.Errorf("placeholder FOO is used but no --foo wordlist provided")
			}
		}

		if usesBAR {
			if flagFuzzBar == "" {
				return fmt.Errorf("placeholder BAR is used but no --bar wordlist provided")
			}
		}

		if usesBUZZ {
			if flagFuzzBuzz == "" {
				return fmt.Errorf("placeholder BUZZ is used but no --buzz wordlist provided")
			}
		}

		if flagFuzzExcludeStat != "" {
			if _, err := status.Parse(flagFuzzExcludeStat); err != nil {
				return fmt.Errorf("invalid exclude-status: %w", err)
			}
		}

		if flagFuzzIncSize != "" {
			if _, err := size.Parse(flagFuzzIncSize); err != nil {
				return fmt.Errorf("invalid include-size: %w", err)
			}
		}
		if flagFuzzExcSize != "" {
			if _, err := size.Parse(flagFuzzExcSize); err != nil {
				return fmt.Errorf("invalid exclude-size: %w", err)
			}
		}

		if flagFuzzDelay != "" {
			if _, err := time.ParseDuration(flagFuzzDelay); err != nil {
				return fmt.Errorf("invalid delay: %w", err)
			}
		}

		if cmd.Flags().Changed("rate") && flagFuzzRate <= 0 {
			return fmt.Errorf("rate must be greater than 0")
		}

		if flagFuzzOutput != "" {
			if fi, err := os.Stat(flagFuzzOutput); err == nil && fi.IsDir() {
				return fmt.Errorf("--output %q is a directory; provide a file path", flagFuzzOutput)
			}
		}

		if cmd.Flags().Changed("format") {
			if _, err := output.Parse(flagFuzzFormat); err != nil {
				return fmt.Errorf("invalid --format: %w", err)
			}
		}

		if flagFuzzProxy != "" {
			if _, err := url.Parse(flagFuzzProxy); err != nil {
				return fmt.Errorf("invalid proxy URL %q: %w", flagFuzzProxy, err)
			}
		}

		if flagFuzzMatchStatus != "" {
			if _, err := status.Parse(flagFuzzMatchStatus); err != nil {
				return fmt.Errorf("invalid match-status (--mc): %w", err)
			}
		}
		if flagFuzzFilterStatus != "" {
			if _, err := status.Parse(flagFuzzFilterStatus); err != nil {
				return fmt.Errorf("invalid filter-status (--fc): %w", err)
			}
		}
		if flagFuzzMatchSize != "" {
			if _, err := size.Parse(flagFuzzMatchSize); err != nil {
				return fmt.Errorf("invalid match-size (--ms): %w", err)
			}
		}
		if flagFuzzFilterSize != "" {
			if _, err := size.Parse(flagFuzzFilterSize); err != nil {
				return fmt.Errorf("invalid filter-size (--fs): %w", err)
			}
		}
		for _, rx := range flagFuzzMatchRegex {
			if _, err := regexp.Compile(rx); err != nil {
				return fmt.Errorf("invalid match-regex (--mr) %q: %w", rx, err)
			}
		}
		for _, rx := range flagFuzzFilterRegex {
			if _, err := regexp.Compile(rx); err != nil {
				return fmt.Errorf("invalid filter-regex (--fr) %q: %w", rx, err)
			}
		}

		if len(flagFuzzExt) > 0 {
			if _, err := extensions.Parse(flagFuzzExt); err != nil {
				return fmt.Errorf("invalid --ext: %w", err)
			}
		}

		if flagFuzzMaxRedirects < 0 {
			return fmt.Errorf("max-redirects cannot be negative")
		}

		if flagFuzzStrategy != "" {
			strategyLower := strings.ToLower(flagFuzzStrategy)
			if strategyLower != "eager" && strategyLower != "bfs" && strategyLower != "dfs" {
				return fmt.Errorf("invalid --strategy: %q (must be eager, bfs, or dfs)", flagFuzzStrategy)
			}
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		stats.GlobalInstrumentation.Reset()
		atomic.StoreInt32(&stats.GlobalInstrumentation.Enabled, 1)

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		// Initialize default config and HTTP client options
		cfg := config.Default()
		cfg.Threads = flagFuzzThreads
		cfg.Timeout = time.Duration(flagFuzzTimeout) * time.Second
		cfg.Quiet = flagFuzzQuiet
		cfg.Proxy = flagFuzzProxy
		cfg.ShowHeaders = flagFuzzShowHeaders
		cfg.ShowTitle = flagFuzzShowTitle

		if flagFuzzMatchStatus != "" {
			exclude, _ := status.Parse(flagFuzzMatchStatus)
			cfg.Status.Include = exclude
		}
		if flagFuzzFilterStatus != "" {
			exclude, _ := status.Parse(flagFuzzFilterStatus)
			cfg.Status.Exclude = exclude
		} else if flagFuzzExcludeStat != "" {
			exclude, _ := status.Parse(flagFuzzExcludeStat)
			cfg.Status.Exclude = exclude
		}

		if flagFuzzMatchSize != "" {
			inc, _ := size.Parse(flagFuzzMatchSize)
			cfg.IncludeSize = inc
		} else if flagFuzzIncSize != "" {
			inc, _ := size.Parse(flagFuzzIncSize)
			cfg.IncludeSize = inc
		}
		if flagFuzzFilterSize != "" {
			exc, _ := size.Parse(flagFuzzFilterSize)
			cfg.ExcludeSize = exc
		} else if flagFuzzExcSize != "" {
			exc, _ := size.Parse(flagFuzzExcSize)
			cfg.ExcludeSize = exc
		}
		cfg.MatchRegex = flagFuzzMatchRegex
		cfg.FilterRegex = flagFuzzFilterRegex
		cfg.MatchContent = flagFuzzMatchContent
		cfg.FilterContent = flagFuzzFilterContent

		// Apply profiles if specified
		if len(flagFuzzProfiles) > 0 {
			store := profile.NewStore()
			resolved, err := resolver.New(store).Resolve(flagFuzzProfiles)
			if err != nil {
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
				fmt.Fprintf(cmd.ErrOrStderr(), "failed to load profiles: %v\n", err)
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
				var overlay scanProfile.Overlay
				if err := p.Decode(&overlay); err != nil {
					cmd.SilenceErrors = true
					cmd.SilenceUsage = true
					fmt.Fprintf(cmd.ErrOrStderr(), "failed to load profile:\n%v\n", err)
					return fmt.Errorf("decode failed")
				}
				scanProfile.Apply(&cfg, overlay)
			}
		}

		// Apply CLI overrides to ensure they take precedence
		applyFuzzCLIOverrides(cmd, &cfg)

		if testHookConfigApplied != nil {
			testHookConfigApplied(cfg)
		}

		// Propagate resolved values to command execution
		flagFuzzMethod = cfg.Method
		flagFuzzData = cfg.Data
		flagFuzzCookie = cfg.Cookies
		flagFuzzRequestFile = cfg.RequestFile

		var delay time.Duration
		if cfg.Delay > 0 {
			delay = cfg.Delay
		} else if flagFuzzDelay != "" {
			delay, _ = time.ParseDuration(flagFuzzDelay)
		}

		var limiter *rate.Limiter
		if cfg.Rate > 0 {
			limiter = rate.NewLimiter(rate.Limit(cfg.Rate), 1)
		} else if flagFuzzRate > 0 {
			limiter = rate.NewLimiter(rate.Limit(flagFuzzRate), 1)
		}

		// Load auxiliary wordlists
		fooWords, err := loadLines(flagFuzzFoo)
		if err != nil {
			return fmt.Errorf("failed to load FOO wordlist: %w", err)
		}
		barWords, err := loadLines(flagFuzzBar)
		if err != nil {
			return fmt.Errorf("failed to load BAR wordlist: %w", err)
		}
		buzzWords, err := loadLines(flagFuzzBuzz)
		if err != nil {
			return fmt.Errorf("failed to load BUZZ wordlist: %w", err)
		}

		var headers http.Header
		if flagFuzzRequestFile != "" {
			tmpl, err := requesttemplate.ParseFile(flagFuzzRequestFile, flagFuzzURL)
			if err != nil {
				return err
			}
			flagFuzzURL = tmpl.URL
			flagFuzzMethod = tmpl.Method
			flagFuzzData = string(tmpl.Body)

			headers = make(http.Header)
			for k, values := range tmpl.Headers {
				for _, v := range values {
					headers.Add(k, v)
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

			cookies := requesttemplate.ExtractCookiesFromHeaders(tmpl.Headers)
			if len(cookies) > 0 {
				var cookiePairs []string
				for _, c := range cookies {
					cookiePairs = append(cookiePairs, c.String())
				}
				flagFuzzCookie = strings.Join(cookiePairs, "; ")
			}
		} else {
			cliHeaders, err := parseFuzzHeaderFlags(cfg.Headers)
			if err != nil {
				return err
			}
			headers = cliHeaders
		}

		// Resolve output format
		outFormat := output.FormatText
		if flagFuzzOutput != "" {
			outFormat = output.FormatFromPath(flagFuzzOutput)
		}
		if cmd.Flags().Changed("format") {
			parsedFormat, _ := output.Parse(flagFuzzFormat)
			outFormat = parsedFormat
		}

		var outWriter io.Writer = os.Stdout
		if flagFuzzOutput != "" {
			f, err := os.OpenFile(flagFuzzOutput, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return fmt.Errorf("cannot open output file: %w", err)
			}
			defer f.Close()
			outWriter = f
		}

		if outFormat != output.FormatText && outWriter == os.Stdout {
			cfg.Quiet = true
		}

		totalCandidates := 1
		if flagFuzzWordlist != "" {
			baseCount := countFileLines(flagFuzzWordlist)
			if len(cfg.Extensions) > 0 {
				baseCount *= (1 + len(cfg.Extensions))
			}
			totalCandidates *= baseCount
		}
		if flagFuzzFoo != "" {
			totalCandidates *= countFileLines(flagFuzzFoo)
		}
		if flagFuzzBar != "" {
			totalCandidates *= countFileLines(flagFuzzBar)
		}
		if flagFuzzBuzz != "" {
			totalCandidates *= countFileLines(flagFuzzBuzz)
		}

		targetManager := targets.NewManager(resolvedFuzzTargets)
		globalSummary := targets.NewGlobalSummary(len(resolvedFuzzTargets))

		errExecute := targetManager.Execute(ctx, func(tCtx targets.TargetContext) error {
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
				var placeholders []string
				if flagFuzzWordlist != "" {
					wordlistsCount++
					placeholders = append(placeholders, "FUZZ")
				}
				if flagFuzzFoo != "" {
					wordlistsCount++
					placeholders = append(placeholders, "FOO")
				}
				if flagFuzzBar != "" {
					wordlistsCount++
					placeholders = append(placeholders, "BAR")
				}
				if flagFuzzBuzz != "" {
					wordlistsCount++
					placeholders = append(placeholders, "BUZZ")
				}
				placeholdersStr := fmt.Sprintf("%s (%d)", strings.Join(placeholders, ", "), len(placeholders))

				excludeStatusStr := cfg.Status.Exclude.String()
				if excludeStatusStr == "" {
					excludeStatusStr = "none"
				}

				info := telemetry.ConfigInfo{
					Target:          flagFuzzURL,
					Method:          cfg.Method,
					Workers:         cfg.Threads,
					Strategy:        cfg.FuzzStrategy,
					AdaptiveEnabled: cfg.Adaptive,
					WordlistsCount:  wordlistsCount,
					PrimaryWordlist: flagFuzzWordlist,
					Placeholders:    placeholdersStr,
					HTTPVersion:     "auto",
					FollowRedirects: cfg.FollowRedirects,
					FilterStatus:    excludeStatusStr,
					TotalCandidates: totalCandidates,
					IsFuzz:          true,
					Extensions:      cfg.Extensions,
				}
				if flagDebug {
					telemetry.PrintConfiguration(tm, terminal.OwnerConfiguration, info)
				} else {
					telemetry.PrintNormalConfiguration(tm, terminal.OwnerConfiguration, info)
				}
			}

			// Transition out of Starting, into Running, and hand over to Progress.
			if err := tm.TransitionAndRelease(terminal.PhaseRunning, terminal.OwnerConfiguration); err != nil {
				return err
			}
			if err := tm.AcquireOwner(terminal.OwnerProgress); err != nil {
				return err
			}

			ctx, cancelSig := signals.SetupContext(ctx, stateMgr)
			defer cancelSig()

			collector := stats.NewCollector()

			var progMgr *progress.Manager
			var renderer *progress.ANSIRenderer
			var progCmdChan chan console.Command
			var consoleCtrl *console.Controller

			enableProgress := shouldEnableProgress(cfg, flagFuzzNoProgress)
			interactive := enableProgress && console.IsTerminal(os.Stdin.Fd())

			var progDone chan struct{}
			progCtx, cancelProg := context.WithCancel(ctx)
			defer cancelProg()

			termCtx, cancelTerm := context.WithCancel(ctx)
			defer cancelTerm()

			if enableProgress {
				modeStr := fmt.Sprintf("Fuzz (%s)", strings.ToUpper(cfg.FuzzStrategy))
				renderer = progress.NewANSIRenderer(tm, targetURL, nil, modeStr)
				progMgr = progress.NewManager(tm, collector, renderer, 1*time.Second)
				progMgr.ConfiguredThreads = cfg.Threads
				progMgr.Formatter = fmttr

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
							case console.CommandStop:
								cancelSig()
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

			stateMgr.Transition(state.PhaseRunning)

			// Setup the primary wordlist reader if provided
			var reader wordlist.Reader
			var primaryChan chan string
			if flagFuzzWordlist != "" {
				reader = wordlist.FileReader{Path: flagFuzzWordlist}
				primaryChan = make(chan string, 100)
				go func() {
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
				Method:          strings.ToUpper(flagFuzzMethod),
				BodyTemplate:    flagFuzzData,
				HeaderTemplates: headers,
				CookieTemplate:  flagFuzzCookie,
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
			}

			collector.SetQueuedJobs(int64(totalCandidates))

			runErr := runner.Run(fuzzCtx, cfg.FuzzStrategy, primaryChan, func(r fuzz.Result) {
				collector.DecrementQueuedJobs()
				if r.Accepted {
					engRes := engine.Result{
						URL:        r.URL,
						StatusCode: r.StatusCode,
						Length:     r.Length,
						Accepted:   r.Accepted,
						Err:        r.Err,
						Origin:     "fuzz",
						Title:      r.Title,
						Headers:    r.Headers,
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

			if fmttr != nil {
				_ = fmttr.Close()
			}

			stateMgr.Transition(state.PhaseWaitingWorkers)
			_ = tm.TransitionTo(terminal.PhaseWaitingWorkers)

			stateMgr.Transition(state.PhaseFinalizing)
			_ = tm.TransitionTo(terminal.PhaseFinalizing)

			if enableProgress && renderer != nil {
				cancelProg()
				if progDone != nil {
					<-progDone
				}
			}

			stateMgr.Transition(state.PhaseTerminalShutdown)
			_ = tm.TransitionTo(terminal.PhaseTerminalShutdown)

			if enableProgress && renderer != nil {
				if interactive && consoleCtrl != nil {
					cancelTerm()
					<-consoleCtrl.Done()
				}
				_ = renderer.Close(terminal.OwnerProgress)
			}
			_ = tm.ReleaseOwner(terminal.OwnerProgress)

			stateMgr.Transition(state.PhaseSummary)
			_ = tm.AcquireAndTransition(terminal.OwnerSummary, terminal.PhaseSummary)

			if !cfg.Quiet {
				snap := runner.Collector.Snapshot()

				telemetry.PrintSummary(tm, terminal.OwnerSummary, telemetry.SummaryInfo{
					IsFuzz:          true,
					Strategy:        cfg.FuzzStrategy,
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

		if len(resolvedFuzzTargets) > 1 && !cfg.Quiet {
			fmt.Printf("\n[*] Global Summary:\n    Targets scanned: %d/%d\n    Total Requests: %d\n    Total Discoveries: %d\n    Duration: %s\n",
				globalSummary.TargetsRun, globalSummary.TargetsTotal, globalSummary.TotalJobs, globalSummary.TotalFound, globalSummary.Duration())
		}

		return nil
	},
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

func applyFuzzCLIOverrides(cmd *cobra.Command, cfg *config.Config) {
	if cmd.Flags().Changed("threads") {
		cfg.Threads = flagFuzzThreads
	}
	if cmd.Flags().Changed("timeout") {
		cfg.Timeout = time.Duration(flagFuzzTimeout) * time.Second
	}
	if cmd.Flags().Changed("delay") {
		if d, err := time.ParseDuration(flagFuzzDelay); err == nil {
			cfg.Delay = d
		}
	}
	if cmd.Flags().Changed("rate") {
		cfg.Rate = flagFuzzRate
	}
	if cmd.Flags().Changed("quiet") {
		cfg.Quiet = flagFuzzQuiet
	}
	if cmd.Flags().Changed("proxy") {
		cfg.Proxy = flagFuzzProxy
	}
	if cmd.Flags().Changed("show-headers") {
		cfg.ShowHeaders = flagFuzzShowHeaders
	}
	if cmd.Flags().Changed("show-title") {
		cfg.ShowTitle = flagFuzzShowTitle
	}
	if cmd.Flags().Changed("request") {
		cfg.RequestFile = flagFuzzRequestFile
	}
	if cmd.Flags().Changed("method") {
		cfg.Method = flagFuzzMethod
	}
	if cmd.Flags().Changed("data") {
		cfg.Data = flagFuzzData
	}
	if cmd.Flags().Changed("cookie") {
		cfg.Cookies = flagFuzzCookie
	}

	if cmd.Flags().Changed("mc") {
		inc, _ := status.Parse(flagFuzzMatchStatus)
		cfg.Status.Include = inc
	}
	if cmd.Flags().Changed("fc") {
		exc, _ := status.Parse(flagFuzzFilterStatus)
		cfg.Status.Exclude = exc
	} else if cmd.Flags().Changed("exclude-status") {
		exc, _ := status.Parse(flagFuzzExcludeStat)
		cfg.Status.Exclude = exc
	}

	if cmd.Flags().Changed("ms") {
		inc, _ := size.Parse(flagFuzzMatchSize)
		cfg.IncludeSize = inc
	} else if cmd.Flags().Changed("include-size") {
		inc, _ := size.Parse(flagFuzzIncSize)
		cfg.IncludeSize = inc
	}
	if cmd.Flags().Changed("fs") {
		exc, _ := size.Parse(flagFuzzFilterSize)
		cfg.ExcludeSize = exc
	} else if cmd.Flags().Changed("exclude-size") {
		exc, _ := size.Parse(flagFuzzExcSize)
		cfg.ExcludeSize = exc
	}

	if cmd.Flags().Changed("mr") {
		cfg.MatchRegex = flagFuzzMatchRegex
	}
	if cmd.Flags().Changed("fr") {
		cfg.FilterRegex = flagFuzzFilterRegex
	}
	if cmd.Flags().Changed("mt") {
		cfg.MatchContent = flagFuzzMatchContent
	}
	if cmd.Flags().Changed("ft") {
		cfg.FilterContent = flagFuzzFilterContent
	}
	if cmd.Flags().Changed("follow-redirects") {
		cfg.FollowRedirects = flagFuzzFollowRedirects
	}
	if cmd.Flags().Changed("max-redirects") {
		cfg.MaxRedirects = flagFuzzMaxRedirects
	}
	if cmd.Flags().Changed("wordlist") {
		cfg.Wordlist = flagFuzzWordlist
	}
	if cmd.Flags().Changed("ext") {
		if exts, err := extensions.Parse(flagFuzzExt); err == nil {
			cfg.Extensions = exts
		}
	}
	if cmd.Flags().Changed("strategy") {
		cfg.FuzzStrategy = flagFuzzStrategy
	}
	if cmd.Flags().Changed("adaptive") {
		cfg.Adaptive = flagFuzzAdaptive
	}
}

func init() {
	rootCmd.AddCommand(fuzzCmd)

	fuzzCmd.Flags().StringVarP(&flagFuzzURL, "url", "u", "", "target URL with placeholders (FUZZ, FOO, BAR, BUZZ)")
	fuzzCmd.Flags().StringVar(&flagFuzzURLFile, "url-file", "", "Target URL file (NOT SUPPORTED in fuzz)")
	fuzzCmd.Flags().StringVarP(&flagFuzzWordlist, "wordlist", "w", "", "primary wordlist path (maps to FUZZ)")
	fuzzCmd.Flags().StringSliceVarP(&flagFuzzExt, "ext", "e", nil, "comma-separated extensions or @file")
	fuzzCmd.Flags().StringVar(&flagFuzzFoo, "foo", "", "wordlist path for FOO placeholder")
	fuzzCmd.Flags().StringVar(&flagFuzzBar, "bar", "", "wordlist path for BAR placeholder")
	fuzzCmd.Flags().StringVar(&flagFuzzBuzz, "buzz", "", "wordlist path for BUZZ placeholder")
	fuzzCmd.Flags().StringVarP(&flagFuzzMethod, "method", "X", "GET", "HTTP method")
	fuzzCmd.Flags().StringVarP(&flagFuzzData, "data", "d", "", "POST request data body with placeholders")
	fuzzCmd.Flags().StringSliceVarP(&flagFuzzHeaders, "header", "H", nil, "custom request headers with placeholders (e.g. -H 'X-Header=FUZZ')")
	fuzzCmd.Flags().IntVarP(&flagFuzzThreads, "threads", "t", 32, "number of concurrent worker threads")
	fuzzCmd.Flags().IntVar(&flagFuzzTimeout, "timeout", 10, "request timeout in seconds")
	fuzzCmd.Flags().StringVarP(&flagFuzzExcludeStat, "exclude-status", "x", "404", "comma-separated status codes to exclude")
	fuzzCmd.Flags().StringVar(&flagFuzzIncSize, "include-size", "", "comma-separated exact sizes or ranges to include")
	fuzzCmd.Flags().StringVar(&flagFuzzExcSize, "exclude-size", "", "comma-separated exact sizes or ranges to exclude")
	fuzzCmd.Flags().StringVarP(&flagFuzzOutput, "output", "o", "", "write results to this file (default: stdout)")
	fuzzCmd.Flags().StringVar(&flagFuzzFormat, "format", "text", "explicit output format (text, json, ndjson, csv, markdown)")
	fuzzCmd.Flags().BoolVarP(&flagFuzzQuiet, "quiet", "q", false, "disable status prefix printing in stdout")
	fuzzCmd.Flags().StringVar(&flagFuzzDelay, "delay", "", "delay between requests (e.g. 50ms, 1s)")
	fuzzCmd.Flags().Float64Var(&flagFuzzRate, "rate", 0, "maximum requests per second rate limit")
	fuzzCmd.Flags().BoolVar(&flagFuzzNoProgress, "no-progress", false, "disable progress output")
	fuzzCmd.Flags().StringVarP(&flagFuzzCookie, "cookie", "b", "", "HTTP request cookies with placeholders")
	fuzzCmd.Flags().StringVar(&flagFuzzProxy, "proxy", "", "HTTP proxy URL to use for requests")

	fuzzCmd.Flags().StringVar(&flagFuzzMatchStatus, "mc", "", "match status codes")
	fuzzCmd.Flags().StringVar(&flagFuzzFilterStatus, "fc", "", "filter status codes")
	fuzzCmd.Flags().StringVar(&flagFuzzMatchSize, "ms", "", "match size")
	fuzzCmd.Flags().StringVar(&flagFuzzFilterSize, "fs", "", "filter size")
	fuzzCmd.Flags().StringSliceVar(&flagFuzzMatchRegex, "mr", nil, "match regex")
	fuzzCmd.Flags().StringSliceVar(&flagFuzzFilterRegex, "fr", nil, "filter regex")
	fuzzCmd.Flags().StringSliceVar(&flagFuzzMatchContent, "mt", nil, "match content types")
	fuzzCmd.Flags().StringSliceVar(&flagFuzzFilterContent, "ft", nil, "filter content types")
	fuzzCmd.Flags().BoolVar(&flagFuzzShowHeaders, "show-headers", false, "show response headers in output")
	fuzzCmd.Flags().BoolVar(&flagFuzzShowTitle, "show-title", false, "show HTML titles in output")
	fuzzCmd.Flags().StringVar(&flagFuzzRequestFile, "request", "", "load raw HTTP request template from file")
	fuzzCmd.Flags().StringSliceVarP(&flagFuzzProfiles, "profile", "p", nil, "load one or more predefined fuzz profiles")
	fuzzCmd.Flags().BoolVar(&flagFuzzFollowRedirects, "follow-redirects", false, "follow HTTP redirects")
	fuzzCmd.Flags().IntVar(&flagFuzzMaxRedirects, "max-redirects", 10, "maximum redirect limit")
	fuzzCmd.Flags().StringVarP(&flagFuzzStrategy, "strategy", "s", "eager", "Traversal strategy (eager, bfs, dfs)")
	fuzzCmd.Flags().BoolVar(&flagFuzzAdaptive, "adaptive", false, "enable adaptive fuzzing (prioritization, framework detection, robots.txt, sitemaps)")
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
