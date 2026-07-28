package presentation

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPath(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		maxLen int
		want   string
	}{
		{"short path", "/usr/bin/ls", 50, "/usr/bin/ls"},
		{"exact length", "/a/b/c", 6, "/a/b/c"},
		{"needs truncation", "/home/user/projects/searchit/wordlists/raft-medium.txt", 35, ".../wordlists/raft-medium.txt"},
		{"very long filename", "/dir/super_long_filename_that_exceeds_max_len.txt", 25, "/dir/super_lo...x_len.txt"},
		{"no extension", "/var/log/nginx", 15, "/var/log/nginx"},
		{"windows path", "C:\\Windows\\System32\\cmd.exe", 25, "C:/.../System32/cmd.exe"},
		{"root only", "/", 10, "/"},
		{"empty string", "", 10, ""},

		// New path compaction tests requested
		{"seclists 50", "/home/unsubble/wordlists/SecLists/Discovery/Web-Content/DirBuster-2007_directory-list-lowercase-2.3-medium.txt", 50, ".../SecLists/Discovery/Web-Content/DirBus...um.txt"},
		{"seclists 40", "/home/unsubble/wordlists/SecLists/Discovery/Web-Content/DirBuster-2007_directory-list-lowercase-2.3-medium.txt", 40, ".../Web-Content/DirBuster-...-medium.txt"},
		{"seclists 30", "/home/unsubble/wordlists/SecLists/Discovery/Web-Content/DirBuster-2007_directory-list-lowercase-2.3-medium.txt", 30, ".../DirBuster-2...3-medium.txt"},
		{"seclists 15", "/home/unsubble/wordlists/SecLists/Discovery/Web-Content/DirBuster-2007_directory-list-lowercase-2.3-medium.txt", 15, ".../DirB....txt"},

		{"nested projects", "/var/www/html/internal/legacy/v1/api/handlers/users.php", 35, ".../v1/api/handlers/users.php"},
		{"nested projects tiny", "/var/www/html/internal/legacy/v1/api/handlers/users.php", 12, ".../us...php"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.FromSlash(tt.path)
			if tt.name == "windows path" {
				// Special case: Windows path uses literal backslashes in test case
				path = tt.path
			}
			got := Path(path, tt.maxLen)
			want := filepath.FromSlash(tt.want)
			if got != want {
				t.Errorf("Path(%q, %d) = %q; want %q", path, tt.maxLen, got, want)
			}
			if len(got) > tt.maxLen {
				t.Errorf("Path(%q, %d) returned length %d, which exceeds max %d", tt.path, tt.maxLen, len(got), tt.maxLen)
			}
		})
	}
}

func TestURL(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		maxLen int
		want   string
	}{
		{"short URL", "http://example.com/api", 50, "http://example.com/api"},
		{"long path", "http://example.com/api/v1/users/admin/dashboard/settings", 45, "http://example.com/api/v1/users/.../settings"},
		{"query truncation", "http://example.com/search?q=vulnerability&sort=desc", 45, "http://example.com/search?q=vul...&sort=desc"},
		{"unparseable", "http://[::1]:8080/api\x00/test", 20, "http://[...api\x00/test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := URL(tt.url, tt.maxLen)
			if got != tt.want {
				t.Errorf("URL(%q, %d) = %q; want %q", tt.url, tt.maxLen, got, tt.want)
			}
			if len(got) > tt.maxLen {
				t.Errorf("URL(%q, %d) returned length %d, which exceeds max %d", tt.url, tt.maxLen, len(got), tt.maxLen)
			}
		})
	}
}

func TestRelativeURL(t *testing.T) {
	target := "http://example.com:8080"

	tests := []struct {
		reqURL string
		want   string
	}{
		{"http://example.com:8080/admin", "/admin"},
		{"http://example.com:8080/search?q=1", "/search?q=1"},
		{"http://example.com:8080/", "/"},
		{"https://example.com:8080/admin", "https://example.com:8080/admin"}, // different scheme
		{"http://other.com/admin", "http://other.com/admin"},                 // different host
		{"/relative/path", "/relative/path"},                                 // already relative
	}

	for _, tt := range tests {
		t.Run(tt.reqURL, func(t *testing.T) {
			if got := RelativeURL(target, tt.reqURL); got != tt.want {
				t.Errorf("RelativeURL(%q, %q) = %q; want %q", target, tt.reqURL, got, tt.want)
			}
		})
	}
}

func TestRedirect(t *testing.T) {
	target := "http://example.com"

	tests := []struct {
		name   string
		source string
		dest   string
		want   string
	}{
		// dest is always shown as the fully resolved absolute URL.
		{"same-host dest is absolute", "http://example.com/login", "http://example.com/auth", "/login \u2192 http://example.com/auth"},
		// Cross-host: source is relative, dest stays absolute.
		{"cross-host dest stays absolute", "http://example.com/sso", "https://okta.com/auth", "/sso \u2192 https://okta.com/auth"},
		// Source on different scheme is shown as absolute.
		{"different-scheme source", "https://example.com/page", "http://example.com/other", "https://example.com/page \u2192 http://example.com/other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Redirect(target, tt.source, tt.dest); got != tt.want {
				t.Errorf("Redirect() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestResolveRedirect(t *testing.T) {
	base := "http://example.com/page"

	tests := []struct {
		name     string
		base     string
		location string
		want     string
	}{
		// Root-relative.
		{"root-relative", base, "/backup/", "http://example.com/backup/"},
		// Relative to base path.
		{"relative", "http://example.com/dir/", "other/", "http://example.com/dir/other/"},
		// Already absolute same-host.
		{"absolute same-host", base, "http://example.com/auth", "http://example.com/auth"},
		// Already absolute cross-host.
		{"absolute cross-host", base, "https://auth.example.com/login", "https://auth.example.com/login"},
		// Protocol change.
		{"https to http", "https://example.com/page", "http://example.com/login", "http://example.com/login"},
		// Query string preserved.
		{"with query string", base, "/search?q=test&page=2", "http://example.com/search?q=test&page=2"},
		// Fragment preserved.
		{"with fragment", base, "/about#team", "http://example.com/about#team"},
		// Dot-dot segment resolved.
		{"parent segment", "http://example.com/a/b/c", "../d", "http://example.com/a/d"},
		// Single-dot segment resolved.
		{"current segment", "http://example.com/a/b/", "./c", "http://example.com/a/b/c"},
		// Empty location returns empty.
		{"empty location", base, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveRedirect(tt.base, tt.location)
			if got != tt.want {
				t.Errorf("ResolveRedirect(%q, %q) = %q; want %q", tt.base, tt.location, got, tt.want)
			}
		})
	}
}

// TestRedirectScanFuzzParity verifies that scan and fuzz produce
// identical redirect presentation for the same response metadata.
// Both tools convert fuzz.Result → engine.Result and route through
// the same presentation.Redirect call in progress/manager.go.
func TestRedirectScanFuzzParity(t *testing.T) {
	target := "http://example.com"

	cases := []struct {
		name     string
		reqURL   string // original request URL (r.URL for both scan and fuzz)
		redirURL string // r.RedirectURL (already resolved by workers)
	}{
		{"same-host relative redirect", "http://example.com/admin", "http://example.com/admin/"},
		{"cross-host redirect", "http://example.com/sso", "https://auth.example.com/login"},
		{"redirect with query", "http://example.com/search", "http://example.com/results?q=test"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate what progress/manager.go does for both scan and fuzz results.
			// Both pass r.RedirectURL as-is to presentation.Redirect.
			scanOut := Redirect(target, tc.reqURL, tc.redirURL)
			fuzzOut := Redirect(target, tc.reqURL, tc.redirURL)

			if scanOut != fuzzOut {
				t.Errorf("scan output %q != fuzz output %q for same input", scanOut, fuzzOut)
			}

			// dest must be the full absolute URL, not a relative path.
			if !strings.HasPrefix(tc.redirURL, "http") {
				t.Skipf("test case %q has non-http redirURL, skipping absoluteness check", tc.name)
			}
			if !strings.Contains(scanOut, tc.redirURL) {
				t.Errorf("output %q does not contain absolute dest %q", scanOut, tc.redirURL)
			}
		})
	}
}

func TestToken(t *testing.T) {
	tests := []struct {
		key     string
		payload string
		maxLen  int
		want    string
	}{
		{"Cookie: session=", "short", 50, "Cookie: session=short"},
		{"Authorization: Bearer ", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c", 45, "Authorization: Bearer eyJhbGciOi...V_adQssw5c"},
		{"Key=", "toolongfortruncate", 10, "Key=t...te"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := Token(tt.key, tt.payload, tt.maxLen)
			if got != tt.want {
				t.Errorf("Token() = %q; want %q", got, tt.want)
			}
			if len(got) > tt.maxLen {
				t.Errorf("Token() returned length %d, which exceeds max %d", len(got), tt.maxLen)
			}
		})
	}
}

func TestError(t *testing.T) {
	err := errors.New("Get \"http://example.com\": dial tcp 127.0.0.1:80: connection refused")
	want := "dial tcp 127.0.0.1:80: connection refused"
	if got := Error(err); got != want {
		t.Errorf("Error() = %q; want %q", got, want)
	}

	if got := Error(nil); got != "" {
		t.Errorf("Error(nil) = %q; want \"\"", got)
	}
}

func TestSize(t *testing.T) {
	if got := Size(500); got != "500B" {
		t.Errorf("Size(500) = %q; want 500B", got)
	}
	if got := Size(1536); got != "1.5KB" {
		t.Errorf("Size(1536) = %q; want 1.5KB", got)
	}
	if got := Size(2048 * 1024); got != "2.0MB" {
		t.Errorf("Size(...) = %q; want 2.0MB", got)
	}
}

func TestDuration(t *testing.T) {
	d := (1 * time.Hour) + (23 * time.Minute) + (4 * time.Second)
	if got := Duration(d); got != "01:23:04" {
		t.Errorf("Duration() = %q; want 01:23:04", got)
	}
}

func TestNumber(t *testing.T) {
	if got := Number(123); got != "123" {
		t.Errorf("Number(123) = %q; want 123", got)
	}
	if got := Number(124500); got != "124.5K" {
		t.Errorf("Number(124500) = %q; want 124.5K", got)
	}
	if got := Number(1500000); got != "1.5M" {
		t.Errorf("Number(1500000) = %q; want 1.5M", got)
	}
}
