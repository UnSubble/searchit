package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func runIntegrationCommand(args []string) (string, error) {
	rootCmd.SetContext(nil)
	scanCmd.SetContext(nil)
	// Reset all flag variables to default
	flagURL = ""
	flagURLFile = ""
	flagWordlist = ""
	flagThreads = 32
	flagTimeout = 10
	flagRecursive = false
	flagMaxDepth = 3
	flagStrategy = "bfs"
	flagExcludeStatus = "404"
	flagRecurseOn = "200,301,302,403"
	flagNormalizePaths = false
	flagCollapseSlashes = false
	flagOutput = "text"
	flagQuiet = false
	flagIncludeSize = ""
	flagExcludeSize = ""
	flagIncludeHeaders = nil
	flagExcludeHeaders = nil
	flagDelay = ""
	resolvedTargets = nil

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	scanCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.SetArgs(args)

	// Capture stdout using pipe
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	oldStdout := os.Stdout
	os.Stdout = w

	// Set buffers for cobra command
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Run command
	execErr := cmd.ExecuteContext(context.Background())

	w.Close()
	os.Stdout = oldStdout

	var stdoutBuf bytes.Buffer
	if _, err := io.Copy(&stdoutBuf, r); err != nil {
		return "", err
	}
	return stdoutBuf.String(), execErr
}

func TestIntegration_Scans(t *testing.T) {
	// Start a mock HTTP server that handles directory fuzzer routes
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "":
			w.WriteHeader(http.StatusOK)
		case "/admin":
			w.Header().Set("Server", "nginx")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("admin area"))
		case "/login":
			w.Header().Set("Server", "Apache")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("forbidden"))
		case "/redirect":
			w.WriteHeader(http.StatusMovedPermanently)
		case "/recurse":
			w.WriteHeader(http.StatusOK)
		case "/recurse/admin":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	// Create a temporary wordlist file containing paths to test
	tmpDir := t.TempDir()
	wordlistPath := filepath.Join(tmpDir, "wordlist.txt")
	wordlistContent := "admin\nlogin\nredirect\nnonexistent\n"
	if err := os.WriteFile(wordlistPath, []byte(wordlistContent), 0600); err != nil {
		t.Fatalf("failed to write wordlist file: %v", err)
	}

	t.Run("basic single target text output", func(t *testing.T) {
		out, err := runIntegrationCommand([]string{"scan", "-u", srv.URL, "-w", wordlistPath})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}

		if !strings.Contains(out, fmt.Sprintf("[*] Target: %s", srv.URL)) {
			t.Errorf("expected target startup message, got:\n%s", out)
		}
		if !strings.Contains(out, "[+] 200 - "+srv.URL+"/admin") {
			t.Errorf("expected admin result in output, got:\n%s", out)
		}
		if !strings.Contains(out, "[+] 403 - "+srv.URL+"/login") {
			t.Errorf("expected login result in output, got:\n%s", out)
		}
		if !strings.Contains(out, "[+] 301 - "+srv.URL+"/redirect") {
			t.Errorf("expected redirect result in output, got:\n%s", out)
		}
		// 404 should be excluded by default
		if strings.Contains(out, "404") {
			t.Errorf("output contains excluded 404 results, got:\n%s", out)
		}
	})

	t.Run("multi-target scan text output", func(t *testing.T) {
		out, err := runIntegrationCommand([]string{"scan", "-u", srv.URL + "," + srv.URL, "-w", wordlistPath})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}

		targetPrints := strings.Count(out, "[*] Target: "+srv.URL)
		if targetPrints != 2 {
			t.Errorf("expected target to be printed 2 times, got %d. Output:\n%s", targetPrints, out)
		}
	})

	t.Run("url file behavior", func(t *testing.T) {
		urlFilePath := filepath.Join(tmpDir, "urls.txt")
		urlFileContent := fmt.Sprintf("# comment\n  \n  %s  \n", srv.URL)
		if err := os.WriteFile(urlFilePath, []byte(urlFileContent), 0600); err != nil {
			t.Fatalf("failed to write url file: %v", err)
		}

		out, err := runIntegrationCommand([]string{"scan", "--url-file", urlFilePath, "-w", wordlistPath})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}

		if !strings.Contains(out, fmt.Sprintf("[*] Target: %s", srv.URL)) {
			t.Errorf("expected target from file, got:\n%s", out)
		}
	})

	t.Run("quiet mode single target", func(t *testing.T) {
		out, err := runIntegrationCommand([]string{"scan", "-u", srv.URL, "-w", wordlistPath, "-q"})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}

		if strings.Contains(out, "[+]") {
			t.Errorf("expected no status prefix, got:\n%s", out)
		}
		if !strings.Contains(out, srv.URL+"/admin\n") {
			t.Errorf("expected raw URL in output, got:\n%s", out)
		}
	})

	t.Run("JSON output multi-target", func(t *testing.T) {
		out, err := runIntegrationCommand([]string{"scan", "-u", srv.URL + "," + srv.URL, "-w", wordlistPath, "--output", "json"})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}

		var parsed []map[string]any
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("failed to parse JSON output: %v. Output:\n%s", err, out)
		}

		// Each target finds admin, login, redirect (3 results), total 6 results.
		if len(parsed) != 6 {
			t.Errorf("expected 6 total results in unified JSON, got %d", len(parsed))
		}
	})

	t.Run("NDJSON output multi-target", func(t *testing.T) {
		out, err := runIntegrationCommand([]string{"scan", "-u", srv.URL + "," + srv.URL, "-w", wordlistPath, "--output", "ndjson"})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != 6 {
			t.Errorf("expected 6 lines of NDJSON, got %d", len(lines))
		}

		for i, line := range lines {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(line), &parsed); err != nil {
				t.Fatalf("line %d is invalid JSON: %v", i+1, err)
			}
		}
	})

	t.Run("recursive + multiple targets", func(t *testing.T) {
		recurseWordlist := filepath.Join(tmpDir, "recurse_wl.txt")
		if err := os.WriteFile(recurseWordlist, []byte("recurse\nadmin\n"), 0600); err != nil {
			t.Fatalf("failed to write recurse wordlist: %v", err)
		}

		out, err := runIntegrationCommand([]string{
			"scan",
			"-u", srv.URL + "," + srv.URL,
			"-w", recurseWordlist,
			"-r",
			"--max-depth", "2",
		})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}

		// /recurse (depth 0) and /recurse/admin (depth 1) should be found on both targets
		recurseAdminHits := strings.Count(out, "/recurse/admin")
		if recurseAdminHits != 2 {
			t.Errorf("expected 2 recursion results for /recurse/admin, got %d. Output:\n%s", recurseAdminHits, out)
		}
	})

	t.Run("quiet mode + multiple targets", func(t *testing.T) {
		out, err := runIntegrationCommand([]string{"scan", "-u", srv.URL + "," + srv.URL, "-w", wordlistPath, "-q"})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}

		if strings.Contains(out, "[+]") {
			t.Errorf("expected no status prefix in quiet mode, got:\n%s", out)
		}
		adminHits := strings.Count(out, srv.URL+"/admin")
		if adminHits != 2 {
			t.Errorf("expected 2 hits of admin, got %d", adminHits)
		}
	})

	t.Run("per-worker request delay", func(t *testing.T) {
		// Use a wordlist with 2 paths and a worker pool of 1.
		// Set delay of 50ms. Total execution should take at least 100ms.
		delayWordlist := filepath.Join(tmpDir, "delay_wl.txt")
		if err := os.WriteFile(delayWordlist, []byte("admin\nlogin\n"), 0600); err != nil {
			t.Fatalf("failed to write delay wordlist: %v", err)
		}

		importTime := time.Now()
		_, err := runIntegrationCommand([]string{
			"scan",
			"-u", srv.URL,
			"-w", delayWordlist,
			"-t", "1",
			"--delay", "50ms",
		})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}
		elapsed := time.Since(importTime)

		if elapsed < 100*time.Millisecond {
			t.Errorf("expected elapsed time >= 100ms with 50ms delay, got %v", elapsed)
		}
	})
}

func TestIntegration_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing url file",
			args:    []string{"scan", "--url-file", "nonexistent_file_xyz.txt"},
			wantErr: "failed to read url file",
		},
		{
			name:    "invalid delay duration string",
			args:    []string{"scan", "-u", "http://localhost", "--delay", "abc"},
			wantErr: "invalid delay",
		},
		{
			name:    "invalid strategy",
			args:    []string{"scan", "-u", "http://localhost", "-r", "--strategy", "invalid"},
			wantErr: "invalid strategy",
		},
		{
			name:    "max-depth without recursive",
			args:    []string{"scan", "-u", "http://localhost", "--max-depth", "5"},
			wantErr: "max-depth requires recursive scanning",
		},
		{
			name:    "strategy without recursive",
			args:    []string{"scan", "-u", "http://localhost", "--strategy", "bfs"},
			wantErr: "strategy requires recursive scanning",
		},
		{
			name:    "recurse-on without recursive",
			args:    []string{"scan", "-u", "http://localhost", "--recurse-on", "200"},
			wantErr: "recurse-on requires recursive scanning",
		},
		{
			name:    "invalid recurse-on status filters",
			args:    []string{"scan", "-u", "http://localhost", "-r", "--recurse-on", "abc"},
			wantErr: "invalid recurse-on",
		},
		{
			name:    "invalid output format",
			args:    []string{"scan", "-u", "http://localhost", "--output", "invalid"},
			wantErr: "invalid output format",
		},
		{
			name:    "invalid include size format",
			args:    []string{"scan", "-u", "http://localhost", "--include-size", "abc"},
			wantErr: "invalid include-size",
		},
		{
			name:    "invalid exclude size range",
			args:    []string{"scan", "-u", "http://localhost", "--exclude-size", "200-100"},
			wantErr: "invalid exclude-size",
		},
		{
			name:    "invalid include header equal format",
			args:    []string{"scan", "-u", "http://localhost", "--include-header", "Server"},
			wantErr: "invalid include-header",
		},
		{
			name:    "invalid exclude header equal format",
			args:    []string{"scan", "-u", "http://localhost", "--exclude-header", "="},
			wantErr: "invalid exclude-header",
		},
		{
			name:    "invalid threads",
			args:    []string{"scan", "-u", "http://localhost", "-t", "0"},
			wantErr: "threads must be at least 1",
		},
		{
			name:    "invalid max depth",
			args:    []string{"scan", "-u", "http://localhost", "-r", "--max-depth", "0"},
			wantErr: "max depth must be at least 1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runIntegrationCommand(tc.args)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}
