package presentation

import (
	"errors"
	"path/filepath"
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
		// Same-host: both source and dest become relative paths.
		{"internal to internal", "http://example.com/login", "http://example.com/auth", "/login \u2192 /auth"},
		// Cross-host: source is relative, dest remains absolute.
		{"internal to external", "http://example.com/sso", "https://okta.com/auth", "/sso \u2192 https://okta.com/auth"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Redirect(target, tt.source, tt.dest); got != tt.want {
				t.Errorf("Redirect() = %q; want %q", got, tt.want)
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
