package cmd

import (
	"fmt"
	"os"

	"github.com/unsubble/searchit/internal/color"
	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/version"

	"github.com/spf13/cobra"
)

var (
	cfgFile   string
	verbose   bool
	flagDebug bool
	flagColor string
)

var rootCmd = &cobra.Command{
	Use:     "searchit",
	Short:   "Fast and smart content discovery tool",
	Version: version.Version,
	Long: `Searchit is a high-performance directory and file discovery tool.

It combines the simplicity of dirsearch with modern concurrency,
profiles, technology-aware mutations and smart enumeration.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Lookup("color") != nil {
			c, _ := cmd.Flags().GetString("color")
			if c != "" && !color.IsValidMode(c) {
				return fmt.Errorf("invalid --color option %q: must be one of auto, always, never", c)
			}
		}
		return nil
	},
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

	rootCmd.SetVersionTemplate(version.String() + "\n")
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	scanCmd, _ := NewScanCmd()
	rootCmd.AddCommand(scanCmd)

	fuzzCmd, _ := NewFuzzCmd()
	rootCmd.AddCommand(fuzzCmd)

	rootCmd.PersistentFlags().StringVarP(
		&cfgFile,
		"config",
		"c",
		"",
		"config file (default: $XDG_CONFIG_HOME/searchit/config.yaml or ~/.config/searchit/config.yaml)",
	)

	rootCmd.PersistentFlags().BoolVarP(
		&verbose,
		"verbose",
		"v",
		false,
		"verbose output",
	)

	rootCmd.PersistentFlags().BoolVar(
		&flagDebug,
		"debug",
		false,
		"enable debug logging",
	)

	rootCmd.PersistentFlags().StringVar(
		&flagColor,
		"color",
		"auto",
		"control colored output (auto, always, never)",
	)
}
