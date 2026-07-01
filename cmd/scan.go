package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/wordlist"
)

var (
	flagURL           string
	flagWordlist      string
	flagThreads       int
	flagTimeout       int
	flagRecursive     bool
	flagMaxDepth      uint16
	flagStrategy      string
	flagExcludeStatus string
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan a target URL",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if flagURL == "" {
			return fmt.Errorf("flag --url is required")
		}

		if flagThreads < 1 {
			return fmt.Errorf("threads must be at least 1")
		}

		_, err := recursion.ParseStrategy(flagStrategy)
		if err != nil {
			return err
		}

		if cmd.Flags().Changed("max-depth") && !flagRecursive {
			return fmt.Errorf("max-depth requires recursive scanning to be enabled")
		}

		if flagMaxDepth < 1 {
			return fmt.Errorf("max-depth must be at least 1")
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

		cfg := config.Config{
			URL:       flagURL,
			Wordlist:  flagWordlist,
			Threads:   flagThreads,
			Timeout:   flagTimeout,
			Recursive: flagRecursive,
			MaxDepth:  flagMaxDepth,
			Strategy:  strat,
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

		if cfg.Recursive {
			seeds := []string{cfg.URL}
			manager := recursion.NewManager(appState.HTTPClient, appState.Config.Status.Exclude, reader, cfg.Strategy, cfg.MaxDepth)
			results := manager.Run(ctx, seeds, cfg.Threads)
			for r := range results {
				if r.Accepted {
					fmt.Printf("[+] %d - %s\n", r.StatusCode, r.URL)
				}
			}
		} else {
			jobs := make(chan engine.Job, cfg.Threads)
			results := engine.Start(ctx, appState.HTTPClient, appState.Config.Status.Exclude, cfg.Threads, jobs)

			go func() {
				p := wordlist.Producer{
					BaseURL: cfg.URL,
					Reader:  reader,
				}
				_ = p.Produce(ctx, jobs)
			}()

			for r := range results {
				if r.Accepted {
					fmt.Printf("[+] %d - %s\n", r.StatusCode, r.URL)
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

	_ = scanCmd.MarkFlagRequired("url")
}
