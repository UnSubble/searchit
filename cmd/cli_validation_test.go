package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_RedirectValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "negative max-redirects",
			args:    []string{"scan", "-u", "http://localhost", "--follow-redirects", "--max-redirects", "-5"},
			wantErr: true,
			errMsg:  "max-redirects cannot be negative",
		},
		{
			name:    "invalid max-redirects string value",
			args:    []string{"scan", "-u", "http://localhost", "--follow-redirects", "--max-redirects", "abc"},
			wantErr: true,
			errMsg:  "invalid argument",
		},
		{
			name:    "valid max-redirects",
			args:    []string{"scan", "-u", "http://localhost", "--follow-redirects", "--max-redirects", "5"},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, err := runIntegrationCommandWithStderr(tc.args)
			if tc.wantErr && err == nil {
				t.Error("expected validation error, got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.wantErr && tc.errMsg != "" && !strings.Contains(strings.ToLower(stderr), tc.errMsg) {
				t.Errorf("expected error message containing %q, got: %q", tc.errMsg, stderr)
			}
		})
	}
}

func TestCLI_FuzzValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing url and request in fuzz",
			args:    []string{"fuzz"},
			wantErr: true,
			errMsg:  "target url is required",
		},
		{
			name:    "missing placeholder in fuzz url",
			args:    []string{"fuzz", "-u", "http://localhost/admin"},
			wantErr: true,
			errMsg:  "no placeholders",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, err := runIntegrationCommandWithStderr(tc.args)
			if tc.wantErr && err == nil {
				t.Error("expected validation error, got none")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.wantErr && tc.errMsg != "" && !strings.Contains(strings.ToLower(stderr), tc.errMsg) {
				t.Errorf("expected error message containing %q, got: %q", tc.errMsg, stderr)
			}
		})
	}
}

func TestCLI_OutputFormatters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.txt")
	if err := os.WriteFile(wlPath, []byte("admin\n"), 0600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	formats := []string{"csv", "markdown"}
	for _, fmtName := range formats {
		t.Run(fmtName, func(t *testing.T) {
			out, err := runIntegrationCommand([]string{
				"scan", "-u", srv.URL, "-w", wlPath, "--format", fmtName, "-q",
			})
			if err != nil {
				t.Fatalf("format %s failed: %v", fmtName, err)
			}

			switch fmtName {
			case "csv":
				// CSV output should contain columns (e.g. url,status)
				if !strings.Contains(out, srv.URL+"/admin") {
					t.Errorf("expected CSV to contain result URL, got:\n%s", out)
				}
			case "markdown":
				// Markdown table formatting
				if !strings.Contains(out, "|") {
					t.Errorf("expected Markdown table, got:\n%s", out)
				}
			}
		})
	}
}
