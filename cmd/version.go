package cmd

import (
	"fmt"

	"github.com/unsubble/searchit/internal/update"
	"github.com/unsubble/searchit/internal/version"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Println(version.Long())
		fmt.Println("----------------------------------------")

		mgr := update.NewManager()
		if res, err := mgr.Check(false, "", false); err == nil {
			fmt.Printf("        CURRENT VERSION\n\n                %s\n\n\n", res.CurrentVersion.Original)
			fmt.Printf("        RECOMMENDED\n\n                %s\n\n\n", res.LatestStable.Original)
			fmt.Printf("        NEXT MAJOR\n\n                %s\n\n\n", res.CurrentVersion.NextMajor())
			fmt.Printf("        STATUS\n\n                %s\n\n", res.Status)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
