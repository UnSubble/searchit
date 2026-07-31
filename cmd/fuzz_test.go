package cmd

import (
	"strings"
	"testing"
)

func FuzzHeaderFlagsAndTargetSplitting(f *testing.F) {
	f.Add("Server=nginx")
	f.Add("Server")
	f.Add("=")
	f.Add("http://a.com,http://b.com")

	f.Fuzz(func(t *testing.T, s string) {
		// Fuzz validateHeaderFlag
		err := validateHeaderFlag(s)

		// Fuzz parseHeaderFlags only if it is a valid format to avoid bounds panic
		if err == nil {
			_ = parseHeaderFlags([]string{s})
		}

		// Fuzz target comma splitting logic
		var targets []string
		for _, val := range strings.Split(s, ",") {
			trimmed := strings.TrimSpace(val)
			if trimmed != "" {
				targets = append(targets, trimmed)
			}
		}
		_ = targets
	})
}

func TestFuzzCLI_HTTPVersionValidation(t *testing.T) {
	cmd, _ := NewFuzzCmd()

	validArgs := [][]string{
		{"-u", "http://localhost/FUZZ", "-w", "words.txt", "--http-version", "auto"},
		{"-u", "http://localhost/FUZZ", "-w", "words.txt", "--http-version", "0.9"},
		{"-u", "http://localhost/FUZZ", "-w", "words.txt", "--http-version", "1.0"},
		{"-u", "http://localhost/FUZZ", "-w", "words.txt", "--http-version", "1.1"},
		{"-u", "http://localhost/FUZZ", "-w", "words.txt", "--http-version", "2"},
	}
	for _, args := range validArgs {
		cmd.SetArgs(args)
		if err := cmd.ParseFlags(args); err != nil {
			t.Errorf("failed to parse valid flags %v: %v", args, err)
		}
	}

	invalidArgs := []string{"-u", "http://localhost/FUZZ", "-w", "words.txt", "--http-version", "invalid"}
	cmd.SetArgs(invalidArgs)
	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error for invalid --http-version, got nil")
	}
}
