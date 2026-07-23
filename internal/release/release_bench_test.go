package release

import (
	"github.com/unsubble/searchit/internal/semver"
	"testing"
)

func BenchmarkValidateBump(b *testing.B) {
	analysis := CommitAnalysis{
		Added:      3,
		Fixed:      10,
		Refactored: 2,
		Optimized:  5,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ValidateBump("patch", analysis)
		ValidateBump("minor", analysis)
		ValidateBump("major", analysis)
	}
}

func BenchmarkGenerateNewsPreview(b *testing.B) {
	v, _ := semver.Parse("v1.5.0")
	analysis := CommitAnalysis{
		Added:      3,
		Fixed:      10,
		Refactored: 2,
		Optimized:  5,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		GenerateNewsPreview(v, analysis)
	}
}
