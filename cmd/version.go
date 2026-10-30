package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/color"
	"github.com/unsubble/searchit/internal/update"
	"github.com/unsubble/searchit/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		colorMode := "auto"
		if f := cmd.Flags().Lookup("color"); f != nil && f.Value.String() != "" {
			colorMode = f.Value.String()
		} else if flagColor != "" {
			colorMode = flagColor
		}
		useColor := color.Resolve(colorMode, false, func() bool {
			return color.IsTerminal(os.Stdout)
		})
		c := color.New(useColor)

		cmd.Println(version.Long())
		fmt.Println("----------------------------------------")

		mgr := update.NewManager()
		if res, err := mgr.Check(false, "", false); err == nil {
			fmt.Printf("        %s\n\n                %s\n\n\n", c.Heading("CURRENT VERSION"), res.CurrentVersion.Original)
			fmt.Printf("        %s\n\n                %s\n\n\n", c.Heading("RECOMMENDED"), res.LatestStable.Original)
			fmt.Printf("        %s\n\n                %s\n\n\n", c.Heading("NEXT MAJOR"), res.CurrentVersion.NextMajor())

			statusStr := res.Status
			if c != nil && c.Enabled() {
				switch res.Status {
				case "UP TO DATE":
					statusStr = c.Success(res.Status)
				case "UPDATE AVAILABLE", "ROLLBACK AVAILABLE", "DOWNGRADE REQUESTED":
					statusStr = c.Warning(res.Status)
				default:
					statusStr = c.Warning(res.Status)
				}
			}
			fmt.Printf("        %s\n\n                %s\n\n", c.Heading("STATUS"), statusStr)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
