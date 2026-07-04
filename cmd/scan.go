package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/wordlist"
)

var (
	flagURL             string
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
	flagIncludeSize     string
	flagExcludeSize     string
	flagIncludeHeaders  []string
	flagExcludeHeaders  []string
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

		cfg := config.Config{
			URL:       flagURL,
			Wordlist:  flagWordlist,
			Threads:   flagThreads,
			Timeout:   flagTimeout,
			Recursive: flagRecursive,
			MaxDepth:  flagMaxDepth,
			Strategy:  strat,
			RecurseOn: recurseFilters,
			Paths: config.PathConfig{
				NormalizePaths:  flagNormalizePaths,
				CollapseSlashes: flagCollapseSlashes,
			},
			Output:         flagOutput,
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
			fmttr = output.NewTextFormatter(os.Stdout)
		}
		defer fmttr.Close()

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

			seeds := []string{cfg.URL}
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
				jobs,
			)

			go func() {
				p := wordlist.Producer{
					BaseURL:         cfg.URL,
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

	scanCmd.Flags().Uint16Var(
		&flagMaxDepth,
		"max-depth",
		3,
		"maximum recursion depth",
	)

	scanCmd.Flags().StringVar(
		&flagStrategy,
		"strategy",
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

	scanCmd.Flags().StringVar(
		&flagOutput,
		"output",
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

	_ = scanCmd.MarkFlagRequired("url")
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
