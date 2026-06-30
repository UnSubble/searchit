package wordlist_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/wordlist"
)

func TestJoin(t *testing.T) {
	tests := []struct {
		base string
		path string
		want string
	}{
		// No existing base path
		{"https://example.com", "admin", "https://example.com/admin"},
		{"https://example.com/", "admin", "https://example.com/admin"},
		{"https://example.com", "/admin", "https://example.com/admin"},
		{"https://example.com/", "/admin", "https://example.com/admin"},

		// Trailing slash on word
		{"https://example.com", "admin/", "https://example.com/admin/"},

		// Existing base path preserved
		{"https://example.com/app", "login", "https://example.com/app/login"},
		{"https://example.com/app/", "login", "https://example.com/app/login"},
		{"https://example.com/app", "/login", "https://example.com/app/login"},
		{"https://example.com/app/", "/login", "https://example.com/app/login"},

		// Nested path word
		{"https://example.com", "api/v1", "https://example.com/api/v1"},
		{"https://example.com/app", "api/v1", "https://example.com/app/api/v1"},

		// Empty path is valid — used for base validation in Producer
		{"https://example.com", "", "https://example.com/"},
	}

	for _, tc := range tests {
		t.Run(tc.base+"+"+tc.path, func(t *testing.T) {
			got, err := wordlist.Join(tc.base, tc.path)
			if err != nil {
				t.Fatalf("Join returned error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestJoin_RejectsQueryString(t *testing.T) {
	bases := []string{
		"https://example.com?debug=1",
		"https://example.com?x=1&y=2",
		"https://example.com/app?key=val",
	}
	for _, base := range bases {
		t.Run(base, func(t *testing.T) {
			_, err := wordlist.Join(base, "path")
			if err == nil {
				t.Errorf("Join(%q, ...) returned nil, want error", base)
			}
		})
	}
}

func TestJoin_RejectsFragment(t *testing.T) {
	bases := []string{
		"https://example.com#section",
		"https://example.com/app#top",
	}
	for _, base := range bases {
		t.Run(base, func(t *testing.T) {
			_, err := wordlist.Join(base, "path")
			if err == nil {
				t.Errorf("Join(%q, ...) returned nil, want error", base)
			}
		})
	}
}
