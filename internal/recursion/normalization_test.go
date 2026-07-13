package recursion

import "testing"

func TestNormalizeURL_Correctness(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"http://example.com/path/", "http://example.com/path"},
		{"http://example.com/path/?a=1", "http://example.com/path?a=1"},
		{"http://example.com/path/#frag", "http://example.com/path"},
		{"http://example.com/path/?a=1#frag", "http://example.com/path?a=1"},
		{"http://example.com/", "http://example.com/"},
		{"http://example.com/?a=1", "http://example.com/?a=1"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeURL(tc.input)
			if got != tc.want {
				t.Errorf("normalizeURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
