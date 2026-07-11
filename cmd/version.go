package cmd

import (
	"github.com/unsubble/searchit/internal/version"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Println(version.Long())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
