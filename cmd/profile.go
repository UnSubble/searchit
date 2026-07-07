package cmd

import (
	"github.com/spf13/cobra"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage Searchit profiles",
	Long: `Manage Searchit profiles.

Profiles are global Searchit resources that bundle configuration
for any tool (scan, fuzz, subdomain, workflow, etc.).

Use subcommands to list and inspect available profiles.`,
}

func init() {
	rootCmd.AddCommand(profileCmd)
}
