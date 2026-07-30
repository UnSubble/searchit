package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type FlagGroup struct {
	Title string
	Names []string
}

type HelpConfig struct {
	Examples   string
	Groups     []FlagGroup
	HelpAllCmd string
}

func setupCmdHelp(cmd *cobra.Command, getHelpAll func() bool, config HelpConfig) {
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		w := c.OutOrStdout()
		if getHelpAll != nil && getHelpAll() {
			renderHelpAll(w, c)
		} else {
			renderCondensedHelp(w, c, config)
		}
	})
}

func renderCondensedHelp(w io.Writer, cmd *cobra.Command, cfg HelpConfig) {
	if cmd.Short != "" {
		fmt.Fprintf(w, "%s\n\n", cmd.Short)
	}
	fmt.Fprintf(w, "Usage:\n  %s\n\n", cmd.UseLine())

	if cfg.Examples != "" {
		fmt.Fprintf(w, "Examples:\n%s\n\n", strings.TrimRight(cfg.Examples, "\n"))
	}

	flags := cmd.Flags()

	for _, group := range cfg.Groups {
		groupSet := pflag.NewFlagSet(group.Title, pflag.ContinueOnError)
		for _, name := range group.Names {
			f := flags.Lookup(name)
			if f != nil {
				groupSet.AddFlag(f)
			}
		}
		if groupSet.HasFlags() {
			fmt.Fprintf(w, "%s:\n%s\n", group.Title, groupSet.FlagUsages())
		}
	}

	inheritedFlags := cmd.InheritedFlags()
	if inheritedFlags.HasFlags() {
		fmt.Fprintf(w, "Global Flags:\n%s\n", inheritedFlags.FlagUsages())
	}

	fmt.Fprintf(w, "For the complete list of options, use:\n  %s\n", cfg.HelpAllCmd)
}

func renderHelpAll(w io.Writer, cmd *cobra.Command) {
	if cmd.Short != "" {
		fmt.Fprintf(w, "%s\n\n", cmd.Short)
	}
	fmt.Fprintf(w, "Usage:\n  %s\n\n", cmd.UseLine())

	fmt.Fprintf(w, "All Flags:\n%s\n", cmd.Flags().FlagUsages())

	inheritedFlags := cmd.InheritedFlags()
	if inheritedFlags.HasFlags() {
		fmt.Fprintf(w, "Global Flags:\n%s", inheritedFlags.FlagUsages())
	}
}
