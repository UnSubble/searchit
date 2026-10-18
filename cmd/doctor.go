package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/doctor"
)

const doctorSepWidth = 40

var doctorCmd = &cobra.Command{
	Use:           "doctor",
	Short:         "Run searchit ecosystem health checks",
	Long:          "Diagnose the health of the searchit installation and its dependencies.",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	w := cmd.OutOrStdout()

	fmt.Fprintln(w, "searchit doctor")
	fmt.Fprintln(w)

	d := doctor.NewDoctor()
	summary := d.RunAllChecks()

	for _, r := range summary.Results {
		printCheckResult(w, r)
	}

	fmt.Fprintln(w, strings.Repeat("-", doctorSepWidth))

	switch {
	case summary.HasFail || summary.HasUnverified:
		fmt.Fprintln(w, "STATUS: NOT READY")
	case summary.HasWarn:
		fmt.Fprintln(w, "STATUS: HEALTHY WITH WARNINGS")
	default:
		fmt.Fprintln(w, "STATUS: HEALTHY")
	}

	if summary.HasFail {
		// Print output first, then signal failure via error return.
		// Cobra will print this error message; use a sentinel so the message
		// is readable without duplication from Cobra's error prefix.
		return fmt.Errorf("one or more health checks failed")
	}

	return nil
}

// printCheckResult writes a single check result to w in the format:
//
//	PASS  CHECK NAME
//	      diagnostic message
func printCheckResult(w io.Writer, r doctor.CheckResult) {
	fmt.Fprintf(w, "  %s  %s\n", r.Status, r.Name)

	// Message (may be multi-line for paths list etc.)
	for _, line := range strings.Split(r.Message, "\n") {
		fmt.Fprintf(w, "        %s\n", line)
	}
	fmt.Fprintln(w)
}
