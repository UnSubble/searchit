package semver

import (
	"fmt"
	"testing"
)

func BenchmarkParse(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Parse("v1.23.456-beta.1")
	}
}

func BenchmarkCompare(b *testing.B) {
	v1, _ := Parse("v1.23.456-beta.1")
	v2, _ := Parse("v1.23.456")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v1.Compare(v2)
	}
}

func BenchmarkMassiveDowngradeSimulation(b *testing.B) {
	// Simulate 10,000 comparisons
	v1, _ := Parse("v1.0.0")
	var versions []Version
	for i := 0; i < 10000; i++ {
		v, _ := Parse(fmt.Sprintf("v0.%d.0", i))
		versions = append(versions, v)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, v2 := range versions {
			v1.Compare(v2)
		}
	}
}
