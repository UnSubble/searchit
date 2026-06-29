package status_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/status"
)

// ─────────────────────────────────────────────────────────────────────────────
// ExactPattern
// ─────────────────────────────────────────────────────────────────────────────

func TestExactPattern_Match(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		code  int
		want  bool
	}{
		{"match 200", "200", 200, true},
		{"miss 201", "200", 201, false},
		{"match 404", "404", 404, true},
		{"miss 403", "404", 403, false},
		{"match 503", "503", 503, true},
		{"miss 500", "503", 500, false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, err := status.Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.input, err)
			}
			if got := f.Match(tc.code); got != tc.want {
				t.Errorf("Match(%d) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

func TestExactPattern_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"200", "200"},
		{"404", "404"},
		{"503", "503"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			f, err := status.Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.input, err)
			}
			if got := f.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// WildcardPattern
// ─────────────────────────────────────────────────────────────────────────────

func TestWildcardPattern_Match(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pattern  string
		code     int
		wantMatch bool
	}{
		// 2xx
		{"2xx matches 200", "2xx", 200, true},
		{"2xx matches 250", "2xx", 250, true},
		{"2xx matches 299", "2xx", 299, true},
		{"2xx misses 300", "2xx", 300, false},
		{"2xx misses 199", "2xx", 199, false},

		// 3xx
		{"3xx matches 300", "3xx", 300, true},
		{"3xx matches 301", "3xx", 301, true},
		{"3xx matches 399", "3xx", 399, true},
		{"3xx misses 400", "3xx", 400, false},

		// 40x
		{"40x matches 400", "40x", 400, true},
		{"40x matches 405", "40x", 405, true},
		{"40x matches 409", "40x", 409, true},
		{"40x misses 410", "40x", 410, false},
		{"40x misses 399", "40x", 399, false},

		// 50x
		{"50x matches 500", "50x", 500, true},
		{"50x matches 505", "50x", 505, true},
		{"50x matches 509", "50x", 509, true},
		{"50x misses 510", "50x", 510, false},

		// 5xx
		{"5xx matches 500", "5xx", 500, true},
		{"5xx matches 599", "5xx", 599, true},
		{"5xx misses 600", "5xx", 600, false},
		{"5xx misses 499", "5xx", 499, false},

		// Case-insensitive input
		{"2XX matches 201", "2XX", 201, true},
		{"2Xx matches 202", "2Xx", 202, true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, err := status.Parse(tc.pattern)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.pattern, err)
			}
			if got := f.Match(tc.code); got != tc.wantMatch {
				t.Errorf("Match(%d) = %v, want %v", tc.code, got, tc.wantMatch)
			}
		})
	}
}

func TestWildcardPattern_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"2xx", "2xx"},
		{"3xx", "3xx"},
		{"40x", "40x"},
		{"5xx", "5xx"},
		// Upper-case input normalises to lower-case.
		{"2XX", "2xx"},
		{"40X", "40x"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			f, err := status.Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.input, err)
			}
			if got := f.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RangePattern
// ─────────────────────────────────────────────────────────────────────────────

func TestRangePattern_Match(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		code    int
		want    bool
	}{
		// 100-200
		{"100-200 matches 100", "100-200", 100, true},
		{"100-200 matches 150", "100-200", 150, true},
		{"100-200 matches 200", "100-200", 200, true},
		{"100-200 misses 99",  "100-200", 99, false},
		{"100-200 misses 201", "100-200", 201, false},

		// 201-299
		{"201-299 matches 201", "201-299", 201, true},
		{"201-299 matches 250", "201-299", 250, true},
		{"201-299 matches 299", "201-299", 299, true},
		{"201-299 misses 200", "201-299", 200, false},
		{"201-299 misses 300", "201-299", 300, false},

		// 500-599
		{"500-599 matches 500", "500-599", 500, true},
		{"500-599 matches 550", "500-599", 550, true},
		{"500-599 matches 599", "500-599", 599, true},
		{"500-599 misses 499", "500-599", 499, false},
		{"500-599 misses 600", "500-599", 600, false},

		// 500-503
		{"500-503 matches 502", "500-503", 502, true},
		{"500-503 matches 503", "500-503", 503, true},
		{"500-503 misses 504", "500-503", 504, false},

		// Single-element range
		{"200-200 matches 200", "200-200", 200, true},
		{"200-200 misses 201", "200-200", 201, false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, err := status.Parse(tc.pattern)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.pattern, err)
			}
			if got := f.Match(tc.code); got != tc.want {
				t.Errorf("Match(%d) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

func TestRangePattern_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"100-200", "100-200"},
		{"500-599", "500-599"},
		// Whitespace around '-' is stripped in normalised form.
		{"100 - 200", "100-200"},
		{"500 - 503", "500-503"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			f, err := status.Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.input, err)
			}
			if got := f.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Filters
// ─────────────────────────────────────────────────────────────────────────────

func TestFilters_Match_Composite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		code    int
		want    bool
	}{
		// From the spec examples
		{"2xx,303 matches 200", "2xx,303", 200, true},
		{"2xx,303 matches 250", "2xx,303", 250, true},
		{"2xx,303 matches 303", "2xx,303", 303, true},
		{"2xx,303 misses 404",  "2xx,303", 404, false},

		{"100-200,500-503 matches 150", "100-200,500-503", 150, true},
		{"100-200,500-503 matches 200", "100-200,500-503", 200, true},
		{"100-200,500-503 misses 201",  "100-200,500-503", 201, false},
		{"100-200,500-503 matches 502", "100-200,500-503", 502, true},

		// Additional combinations
		{"200,201,204 matches 200", "200,201,204", 200, true},
		{"200,201,204 matches 201", "200,201,204", 201, true},
		{"200,201,204 matches 204", "200,201,204", 204, true},
		{"200,201,204 misses 202",  "200,201,204", 202, false},

		{"100-199,301,5xx matches 150", "100-199,301,5xx", 150, true},
		{"100-199,301,5xx matches 301", "100-199,301,5xx", 301, true},
		{"100-199,301,5xx matches 503", "100-199,301,5xx", 503, true},
		{"100-199,301,5xx misses 200",  "100-199,301,5xx", 200, false},

		{"403,404,500-503 matches 403", "403,404,500-503", 403, true},
		{"403,404,500-503 matches 501", "403,404,500-503", 501, true},
		{"403,404,500-503 misses 405",  "403,404,500-503", 405, false},

		// Whitespace-tolerant parsing from spec
		{"'403, 404, 5xx' matches 404",  "403, 404, 5xx", 404, true},
		{"'403, 404, 5xx' matches 500",  "403, 404, 5xx", 500, true},
		{"'403, 404, 5xx' misses 200",   "403, 404, 5xx", 200, false},
		{"'100 - 200, 303' matches 150", "100 - 200, 303", 150, true},
		{"'100 - 200, 303' matches 303", "100 - 200, 303", 303, true},
		{"'100 - 200, 303' misses 301",  "100 - 200, 303", 301, false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, err := status.Parse(tc.pattern)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.pattern, err)
			}
			if got := f.Match(tc.code); got != tc.want {
				t.Errorf("Match(%d) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

func TestFilters_Match_Empty(t *testing.T) {
	t.Parallel()

	// An empty Filters should never match anything.
	var f status.Filters
	for _, code := range []int{100, 200, 301, 404, 500, 999} {
		if f.Match(code) {
			t.Errorf("empty Filters.Match(%d) = true, want false", code)
		}
	}
}

func TestFilters_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"200", "200"},
		{"2xx,303", "2xx,303"},
		{"100-200,500-503", "100-200,500-503"},
		// Whitespace is stripped in the normalised form.
		{"403, 404, 5xx", "403,404,5xx"},
		{"100 - 200, 303", "100-200,303"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			f, err := status.Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.input, err)
			}
			if got := f.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Parse – invalid inputs
// ─────────────────────────────────────────────────────────────────────────────

func TestParse_InvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		// Non-numeric / wrong length exact codes
		{"alpha only", "abc"},
		{"two digits", "20"},
		{"four digits", "2000"},

		// Invalid wildcard character
		{"wildcard with non-x letter", "2xy"},

		// Malformed ranges
		{"range missing upper bound", "200-"},
		{"range missing lower bound", "-300"},
		{"range low > high", "500-200"},

		// Wildcards in range bounds
		{"wildcard as range bound low", "2xx-3xx"},
		{"wildcard as range low only",  "2xx-300"},
		{"wildcard as range high only", "200-3xx"},

		// Miscellaneous
		{"empty token in list", "200,,404"},
		{"leading dash", "-200"},
		{"just a dash", "-"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := status.Parse(tc.input)
			if err == nil {
				t.Errorf("Parse(%q) expected error, got nil", tc.input)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Parse – round-trip (String → Parse → String)
// ─────────────────────────────────────────────────────────────────────────────

func TestParse_RoundTrip(t *testing.T) {
	t.Parallel()

	normalized := []string{
		"200",
		"404",
		"2xx",
		"40x",
		"5xx",
		"100-200",
		"500-599",
		"2xx,303",
		"100-200,500-503",
		"403,404,5xx",
	}

	for _, s := range normalized {
		s := s
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			f, err := status.Parse(s)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", s, err)
			}
			if got := f.String(); got != s {
				t.Errorf("round-trip mismatch: String() = %q, want %q", got, s)
			}
		})
	}
}
