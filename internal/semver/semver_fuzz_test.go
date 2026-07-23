package semver

import (
	"testing"
)

func FuzzParse(f *testing.F) {
	// Add seed corpus
	f.Add("v1.0.0")
	f.Add("v0.5.0-alpha.1")
	f.Add("invalid")
	f.Add("v.1.1.1")
	f.Add("v9999999999.999999999.9999999") // Should not panic, but error on atoi bounds

	f.Fuzz(func(t *testing.T, orig string) {
		Parse(orig) // Just ensure it doesn't panic
	})
}
