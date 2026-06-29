package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	url            string
	wordlist       string
	threads        int
	excludeStatus  []string
	maxRecursion   int
	deepRecursive  bool
	forceRecursive bool
	profile        string
	smartMode      bool
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan a target",
	Run: func(cmd *cobra.Command, args []string) {

		fmt.Println("Starting searchit...")
		fmt.Printf("Target: %s\n", url)
		fmt.Printf("Threads: %d\n", threads)
		fmt.Printf("Profile: %s\n", profile)
		fmt.Printf("Smart mode: %v\n", smartMode)

		// TODO:
		// engine.Start(...)
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)

	scanCmd.Flags().StringVarP(
		&url,
		"url",
		"u",
		"",
		"target URL",
	)

	scanCmd.Flags().StringVarP(
		&wordlist,
		"wordlist",
		"w",
		"",
		"wordlist path",
	)

	scanCmd.Flags().IntVarP(
		&threads,
		"threads",
		"t",
		64,
		"number of concurrent workers",
	)

	scanCmd.Flags().StringSliceVarP(
		&excludeStatus,
		"exclude-status",
		"x",
		[]string{"404"},
		"status codes to ignore",
	)

	scanCmd.Flags().IntVarP(
		&maxRecursion,
		"recursion-depth",
		"R",
		0,
		"maximum recursion depth",
	)

	scanCmd.Flags().BoolVar(
		&deepRecursive,
		"deep-recursive",
		false,
		"enable deep recursive scanning",
	)

	scanCmd.Flags().BoolVar(
		&forceRecursive,
		"force-recursive",
		false,
		"force recursion on all discovered paths",
	)

	scanCmd.Flags().StringVar(
		&profile,
		"profile",
		"default",
		"profile to use",
	)

	scanCmd.Flags().BoolVar(
		&smartMode,
		"smart",
		false,
		"enable technology-aware smart scanning",
	)

	scanCmd.MarkFlagRequired("url")
}
