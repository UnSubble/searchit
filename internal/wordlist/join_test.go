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

		// Query strings and fragments on candidate path
		{"https://example.com", "admin?debug=1", "https://example.com/admin?debug=1"},
		{"https://example.com", "admin#section", "https://example.com/admin"},
		{"https://example.com", "admin?debug=1#section", "https://example.com/admin?debug=1"},
		{"https://example.com/app", "admin?debug=1#section", "https://example.com/app/admin?debug=1"},
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

func TestCleanWord(t *testing.T) {
	tests := []struct {
		name     string
		word     string
		norm     bool
		collapse bool
		wantWord string
		wantOk   bool
	}{
		{"dot rejected", ".", false, false, "", false},
		{"dot rejected (norm)", ".", true, true, "", false},
		{"double dot rejected", "..", false, false, "", false},
		{"double dot rejected (norm)", "..", true, true, "", false},

		{"preserved default", "../../admin", false, false, "../../admin", true},
		{"preserved slash default", "admin//api", false, false, "admin//api", true},

		{"normalize path", "././admin", true, false, "admin", true},
		{"normalize sub segments", "a/./b/../c", true, false, "a/c", true},

		{"collapse slashes", "admin////api", false, true, "admin/api", true},
		{"collapse slashes multiple", "a//b///c", false, true, "a/b/c", true},

		{"preserve traversal normalized", "../../admin", true, false, "../../admin", true},
		{"preserve passwd normalized", "../../etc/passwd", true, false, "../../etc/passwd", true},
		{"multiple root transitions", "a/../../b", true, false, "../b", true},
		{"trailing slash clean", "a/b/", true, false, "a/b", true},
		{"slash cleaning and dot", "a/./b/", true, false, "a/b", true},
		{"spaces in path", "  admin  ", true, false, "  admin  ", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotWord, gotOk := wordlist.CleanWord(tc.word, tc.norm, tc.collapse)
			if gotOk != tc.wantOk {
				t.Errorf("CleanWord() ok = %v, wantOk %v", gotOk, tc.wantOk)
			}
			if gotOk && gotWord != tc.wantWord {
				t.Errorf("CleanWord() = %q, want %q", gotWord, tc.wantWord)
			}
		})
	}
}

func TestJoin_Errors(t *testing.T) {
	// 1. Invalid base URL (invalid escape sequence / IPv6 bracket)
	_, err := wordlist.Join("http://[::1", "admin/login")
	if err == nil {
		t.Error("expected error for invalid base URL, got nil")
	}

	// 2. Invalid candidate path (invalid escape sequence / IPv6 bracket)
	_, err = wordlist.Join("http://example.com", "http://[::1")
	if err == nil {
		t.Error("expected error for invalid candidate path, got nil")
	}
}

func FuzzCleanWord(f *testing.F) {
	f.Add(".", true, true)
	f.Add("admin//api", false, true)
	f.Add("././admin", true, false)
	f.Add("a/../../b", true, true)
	f.Add("", false, false)

	f.Fuzz(func(t *testing.T, word string, norm, collapse bool) {
		_, _ = wordlist.CleanWord(word, norm, collapse)
	})
}

func BenchmarkCleanWord(b *testing.B) {
	words := []string{
		"admin/./api/../v1",
		"a//b///c////d",
		"../../etc/passwd",
		"normal-path",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, w := range words {
			_, _ = wordlist.CleanWord(w, true, true)
		}
	}
}
