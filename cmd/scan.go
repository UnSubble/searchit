package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/size"
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

	resolvedTargets []string
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

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		excludeFilters, err := status.Parse(flagExcludeStatus)
		if err != nil {
			return fmt.Errorf("invalid exclude-status: %w", err)
		}

		strat, err := recursion.ParseStrategy(flagStrategy)
		if err != nil {
			return err
		}

		recurseFilters, err := status.Parse(flagRecurseOn)
		if err != nil {
			return fmt.Errorf("invalid recurse-on: %w", err)
		}

		incSize, err := size.Parse(flagIncludeSize)
		if err != nil {
			return err
		}
		excSize, err := size.Parse(flagExcludeSize)
		if err != nil {
			return err
		}

		incHeaders := parseHeaderFlags(flagIncludeHeaders)
		excHeaders := parseHeaderFlags(flagExcludeHeaders)

		var delay time.Duration
		if flagDelay != "" {
			delay, _ = time.ParseDuration(flagDelay)
		}

		var connectTimeout time.Duration
		if flagConnectTimeout != "" {
			connectTimeout, _ = time.ParseDuration(flagConnectTimeout)
		}

		cfg := config.Config{
			URLs:           resolvedTargets,
			Wordlist:       flagWordlist,
			Threads:        flagThreads,
			Timeout:        flagTimeout,
			Delay:          delay,
			Rate:           flagRate,
			ConnectTimeout: connectTimeout,
			Recursive:      flagRecursive,
			MaxDepth:       flagMaxDepth,
			Strategy:       strat,
			RecurseOn:      recurseFilters,
			Paths: config.PathConfig{
				NormalizePaths:  flagNormalizePaths,
				CollapseSlashes: flagCollapseSlashes,
			},
			Output:         flagOutput,
			Quiet:          flagQuiet,
			IncludeSize:    incSize,
			ExcludeSize:    excSize,
			IncludeHeaders: incHeaders,
			ExcludeHeaders: excHeaders,
			Status: config.StatusConfig{
				Exclude: excludeFilters,
			},
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

			if cfg.Recursive {
				if cfg.Output == "text" {
					fmt.Println("[*] Recursive scan enabled")
					fmt.Printf("[*] Strategy: %s\n", cfg.Strategy.String())
					fmt.Printf("[*] Max depth: %d\n", cfg.MaxDepth)
					fmt.Printf("[*] Recurse on: %s\n", flagRecurseOn)
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
				)
				results := manager.Run(ctx, seeds, cfg.Threads)
				for r := range results {
					if r.Accepted {
						_ = fmttr.Print(r)
					}
				}
			} else {
				jobs := make(chan engine.Job, cfg.Threads)
				results := engine.Start(
					ctx,
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
				)

				go func() {
					p := wordlist.Producer{
						BaseURL:         targetURL,
						Reader:          reader,
						NormalizePaths:  cfg.Paths.NormalizePaths,
						CollapseSlashes: cfg.Paths.CollapseSlashes,
					}
					_ = p.Produce(ctx, jobs)
				}()

				for r := range results {
					if r.Accepted {
						_ = fmttr.Print(r)
					}
				}
			}
		}

		return nil
	},
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
		"10s",
		"timeout for establishing new TCP connections",
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
