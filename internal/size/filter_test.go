package size_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/size"
)

func TestFilters_Match(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		val       int64
		wantMatch bool
	}{
		{"exact match", "123", 123, true},
		{"exact mismatch", "123", 124, false},
		{"range match low", "100-200", 100, true},
		{"range match mid", "100-200", 150, true},
		{"range match high", "100-200", 200, true},
		{"range mismatch low", "100-200", 99, false},
		{"range mismatch high", "100-200", 201, false},
		{"multiple match first", "123,456", 123, true},
		{"multiple match second", "123,456", 456, true},
		{"multiple mismatch", "123,456", 200, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := size.MustParse(tc.pattern)
			got := f.Match(tc.val)
			if got != tc.wantMatch {
				t.Errorf("Match(%d) for pattern %q = %v, want %v", tc.val, tc.pattern, got, tc.wantMatch)
			}
		})
	}
}

func BenchmarkSizeMatch(b *testing.B) {
	f := size.MustParse("0,100-200,500,1000-2000")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.Match(150)
		_ = f.Match(300)
	}
}
