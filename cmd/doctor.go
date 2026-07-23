package cmd

import (
	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/doctor"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run searchit ecosystem health checks",
	Run: func(cmd *cobra.Command, args []string) {
		importFmt := true // just to ensure we import fmt correctly if needed
		if importFmt {
			// Actually need to just print
		}

		results, allHealthy := doctor.NewDoctor().RunAllChecks()
		for _, r := range results {
			cmd.Printf("        %s\n\n                %s\n\n\n", r.Name, r.Status)
		}

		cmd.Printf("        STATUS\n\n")
		if allHealthy {
			cmd.Printf("                HEALTHY\n\n")
		} else {
			cmd.Printf("                NOT READY\n\n")
		}
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
