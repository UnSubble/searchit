package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/github"
	"github.com/unsubble/searchit/internal/news"
	"github.com/unsubble/searchit/internal/version"
)

var newsCmd = &cobra.Command{
	Use:   "news [latest|current|vX.Y.Z]",
	Short: "Read release news from the single source of truth",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		targetVersion := version.Version
		if len(args) > 0 {
			switch args[0] {
			case "latest":
				gh := github.NewClient()
				if stable, err := gh.GetLatestStable(); err == nil {
					targetVersion = stable.Original
				} else {
					fmt.Println("UNABLE TO DETERMINE LATEST VERSION.")
					os.Exit(1)
				}
			case "current":
				targetVersion = version.Version
			default:
				targetVersion = args[0]
			}
		}

		n, err := news.Fetch(targetVersion)
		if err != nil {
			fmt.Printf("NEWS SYSTEM ERROR: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("        VERSION\n\n                %s\n\n\n", n.Version)
		fmt.Printf("        NEWS\n\n")
		fmt.Println(n.Content)
	},
}

func init() {
	rootCmd.AddCommand(newsCmd)
}
