package cmd

import (
	"fmt"
	"runtime"

	"github.com/UnSubble/searchit/internal/version"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show build information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s %s\n\n",
			version.Name,
			version.Version,
		)

		fmt.Printf("Commit:      %s\n", version.Commit)
		fmt.Printf("Built at:    %s\n", version.Date)
		fmt.Printf("Go version:  %s\n", runtime.Version())
		fmt.Printf("Platform:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
		fmt.Printf("Repository:  %s\n", version.Repository)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
