package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/unsubble/searchit/cmd"
)

func TestScanHelpOutput(t *testing.T) {
	t.Run("Condensed Help", func(t *testing.T) {
		scanCmd, _ := cmd.NewScanCmd()
		var buf bytes.Buffer
		scanCmd.SetOut(&buf)
		scanCmd.SetArgs([]string{"--help"})

		err := scanCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error executing --help: %v", err)
		}

		output := buf.String()

		// Verify Examples section
		if !strings.Contains(output, "Examples:") || !strings.Contains(output, "searchit scan -u example.com -w raft-small.txt") {
			t.Errorf("expected Examples section in scan --help, got:\n%s", output)
		}

		// Verify flag groups
		for _, group := range []string{"General:", "Discovery:", "HTTP:", "Matching / Filtering:", "Performance:", "Output:"} {
			if !strings.Contains(output, group) {
				t.Errorf("expected group %q in scan --help output", group)
			}
		}

		// Verify footer
		if !strings.Contains(output, "For the complete list of options, use:") || !strings.Contains(output, "searchit scan --help-all") {
			t.Errorf("expected footer in scan --help, got:\n%s", output)
		}

		// Verify non-condensed flags are omitted from condensed help
		if strings.Contains(output, "--connect-timeout") {
			t.Errorf("did not expect --connect-timeout in condensed scan help")
		}
	})

	t.Run("Help All", func(t *testing.T) {
		scanCmd, _ := cmd.NewScanCmd()
		var buf bytes.Buffer
		scanCmd.SetOut(&buf)
		scanCmd.SetArgs([]string{"--help-all"})

		err := scanCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error executing --help-all: %v", err)
		}

		output := buf.String()

		if !strings.Contains(output, "All Flags:") {
			t.Errorf("expected 'All Flags:' in scan --help-all output, got:\n%s", output)
		}

		// Verify footer is NOT in --help-all
		if strings.Contains(output, "For the complete list of options, use:") {
			t.Errorf("did NOT expect footer in scan --help-all output, but got:\n%s", output)
		}

		// Verify every flag exists in help-all
		for _, flag := range []string{"--url", "--wordlist", "--recursive", "--threads", "--timeout", "--connect-timeout", "--help-all"} {
			if !strings.Contains(output, flag) {
				t.Errorf("expected flag %q in scan --help-all output", flag)
			}
		}
	})
}

func TestFuzzHelpOutput(t *testing.T) {
	t.Run("Condensed Help", func(t *testing.T) {
		fuzzCmd, _ := cmd.NewFuzzCmd()
		var buf bytes.Buffer
		fuzzCmd.SetOut(&buf)
		fuzzCmd.SetArgs([]string{"--help"})

		err := fuzzCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error executing --help: %v", err)
		}

		output := buf.String()

		// Verify Examples section
		if !strings.Contains(output, "Examples:") || !strings.Contains(output, "searchit fuzz -u https://example.com/FUZZ -w words.txt") {
			t.Errorf("expected Examples section in fuzz --help, got:\n%s", output)
		}

		// Verify flag groups
		for _, group := range []string{"General:", "HTTP:", "Discovery:", "Matching / Filtering:", "Output:", "Performance:"} {
			if !strings.Contains(output, group) {
				t.Errorf("expected group %q in fuzz --help output", group)
			}
		}

		// Verify footer
		if !strings.Contains(output, "For the complete list of options, use:") || !strings.Contains(output, "searchit fuzz --help-all") {
			t.Errorf("expected footer in fuzz --help, got:\n%s", output)
		}

		// Verify non-condensed flags are omitted from condensed help
		if strings.Contains(output, "--timeout") {
			t.Errorf("did not expect --timeout in condensed fuzz help")
		}
	})

	t.Run("Help All", func(t *testing.T) {
		fuzzCmd, _ := cmd.NewFuzzCmd()
		var buf bytes.Buffer
		fuzzCmd.SetOut(&buf)
		fuzzCmd.SetArgs([]string{"--help-all"})

		err := fuzzCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error executing --help-all: %v", err)
		}

		output := buf.String()

		if !strings.Contains(output, "All Flags:") {
			t.Errorf("expected 'All Flags:' in fuzz --help-all output, got:\n%s", output)
		}

		// Verify footer is NOT in --help-all
		if strings.Contains(output, "For the complete list of options, use:") {
			t.Errorf("did NOT expect footer in fuzz --help-all output, but got:\n%s", output)
		}

		// Verify every flag exists in help-all
		for _, flag := range []string{"--url", "--wordlist", "--foo", "--bar", "--buzz", "--timeout", "--proxy", "--help-all"} {
			if !strings.Contains(output, flag) {
				t.Errorf("expected flag %q in fuzz --help-all output", flag)
			}
		}
	})
}
