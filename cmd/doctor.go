package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/color"
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

	colorMode := "auto"
	if f := cmd.Flags().Lookup("color"); f != nil && f.Value.String() != "" {
		colorMode = f.Value.String()
	} else if flagColor != "" {
		colorMode = flagColor
	}
	useColor := color.Resolve(colorMode, false, func() bool {
		return color.IsTerminal(w)
	})
	c := color.New(useColor)

	fmt.Fprintln(w, c.Heading("searchit doctor"))
	fmt.Fprintln(w)

	d := doctor.NewDoctor()
	summary := d.RunAllChecks()

	for _, r := range summary.Results {
		printCheckResult(w, r, c)
	}

	fmt.Fprintln(w, strings.Repeat("-", doctorSepWidth))

	switch {
	case summary.HasFail || summary.HasUnverified:
		fmt.Fprintf(w, "%s: %s\n", c.Heading("STATUS"), c.Error("NOT READY"))
	case summary.HasWarn:
		fmt.Fprintf(w, "%s: %s\n", c.Heading("STATUS"), c.Warning("HEALTHY WITH WARNINGS"))
	default:
		fmt.Fprintf(w, "%s: %s\n", c.Heading("STATUS"), c.Success("HEALTHY"))
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
func printCheckResult(w io.Writer, r doctor.CheckResult, c *color.Palette) {
	statusStr := r.Status
	if c != nil && c.Enabled() {
		switch r.Status {
		case "PASS":
			statusStr = c.Success(r.Status)
		case "WARN", "NOT VERIFIED":
			statusStr = c.Warning(r.Status)
		case "FAIL":
			statusStr = c.Error(r.Status)
		}
	}
	fmt.Fprintf(w, "  %s  %s\n", statusStr, r.Name)

	// Message (may be multi-line for paths list etc.)
	for _, line := range strings.Split(r.Message, "\n") {
		fmt.Fprintf(w, "        %s\n", line)
	}
	fmt.Fprintln(w)
}
