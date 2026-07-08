package cmd

import (
	"fmt"
	"os"

	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/version"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:     "searchit",
	Short:   "Fast and smart content discovery tool",
	Version: version.Version,
	Long: `Searchit is a high-performance directory and file discovery tool.

It combines the simplicity of dirsearch with modern concurrency,
profiles, technology-aware mutations and smart enumeration.`,
}

func Execute() {
	if err := profile.RegisterBuiltinValidators(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to bootstrap validators: %v\n", err)
		os.Exit(1)
	}
	if err := profile.RegisterBuiltinDecoders(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to bootstrap decoders: %v\n", err)
		os.Exit(1)
	}

	rootCmd.SetVersionTemplate("searchit v" + version.Version + "\n")
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(
		&cfgFile,
		"config",
		"c",
		"",
		"config file",
	)

	rootCmd.PersistentFlags().BoolVarP(
		&verbose,
		"verbose",
		"v",
		false,
		"verbose output",
	)
}
