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
	})
}
