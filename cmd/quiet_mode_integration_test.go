package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestQuietMode_Text(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	wordlistPath := filepath.Join(tmpDir, "wl.txt")
	if err := os.WriteFile(wordlistPath, []byte("admin\nlogin\n"), 0600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	// 1. Quiet mode: results only, no headers/footers
	out, err := runIntegrationCommand([]string{"scan", "-u", srv.URL, "-w", wordlistPath, "-q"})
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}

	// Suppressed markers
	suppressed := []string{"target:", "profiles:", "scanning:", "completed:", "elapsed:", "[*]", "[+]"}
	for _, s := range suppressed {
		if strings.Contains(strings.ToLower(out), s) {
			t.Errorf("Quiet mode output contains forbidden string %q:\n%s", s, out)
		}
	}

	// Discovered results are present
	expected := []string{srv.URL + "/admin", srv.URL + "/login"}
	for _, exp := range expected {
		if !strings.Contains(out, exp) {
			t.Errorf("Quiet mode missing expected result %q:\n%s", exp, out)
		}
	}

	// Exact result format check: must just be the URLs
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Errorf("Expected exactly 2 output lines, got %d:\n%s", len(lines), out)
	}
}

func TestQuietMode_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	wordlistPath := filepath.Join(tmpDir, "wl.txt")
	if err := os.WriteFile(wordlistPath, []byte("admin\n"), 0600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	out, err := runIntegrationCommand([]string{"scan", "-u", srv.URL, "-w", wordlistPath, "--format", "json", "-q"})
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}

	// Verify machine-parsability
	var results []map[string]any
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("JSON output is not machine-parsable: %v. Output:\n%s", err, out)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result in JSON, got %d", len(results))
	}
}

func TestQuietMode_NDJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	wordlistPath := filepath.Join(tmpDir, "wl.txt")
	if err := os.WriteFile(wordlistPath, []byte("admin\nlogin\n"), 0600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	out, err := runIntegrationCommand([]string{"scan", "-u", srv.URL, "-w", wordlistPath, "--format", "ndjson", "-q"})
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("Expected 2 lines of NDJSON, got %d", len(lines))
	}

	for _, line := range lines {
		var res map[string]any
		if err := json.Unmarshal([]byte(line), &res); err != nil {
			t.Fatalf("NDJSON line is not machine-parsable: %v. Line: %q", err, line)
		}
	}
}

func TestQuietMode_SuppressesProfilesAndRecursionAndAdaptive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	wordlistPath := filepath.Join(tmpDir, "wl.txt")
	if err := os.WriteFile(wordlistPath, []byte("admin\n"), 0600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	out, err := runIntegrationCommand([]string{
		"scan",
		"-u", srv.URL,
		"-w", wordlistPath,
		"--profile", "scan/quick",
		"-r",
		"--adaptive",
		"-q",
	})
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}

	// Output must not contain profile, recursion or target details
	forbidden := []string{"profile", "quick", "recursive", "strategy", "max depth", "target", "adaptive"}
	for _, f := range forbidden {
		if strings.Contains(strings.ToLower(out), f) {
			t.Errorf("Quiet mode printed forbidden info string %q:\n%s", f, out)
		}
	}
}

func runIntegrationCommandWithStderr(args []string) (string, string, error) {
	rootCmd.SetContext(context.Background())
	scanCmd.SetContext(context.Background())
	resetFlagsForTest()

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	scanCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.SetArgs(args)

	// Capture stdout using pipe
	r, w, err := os.Pipe()
	if err != nil {
		return "", "", err
	}
	oldStdout := os.Stdout
	os.Stdout = w

	// Set buffers for cobra command error/out
	errBuf := new(bytes.Buffer)
	cmd.SetOut(errBuf)
	cmd.SetErr(errBuf)

	// Run command
	execErr := cmd.ExecuteContext(context.Background())

	w.Close()
	os.Stdout = oldStdout

	var stdoutBuf bytes.Buffer
	if _, err := io.Copy(&stdoutBuf, r); err != nil {
		return "", "", err
	}
	return stdoutBuf.String(), errBuf.String(), execErr
}

func TestQuietMode_FatalErrors(t *testing.T) {
	// Missing URL should cause a fatal validation error on stderr
	stdout, stderr, err := runIntegrationCommandWithStderr([]string{"scan", "-q"})
	if err == nil {
		t.Fatal("expected scan to fail without target URL, but it succeeded")
	}

	if stdout != "" {
		t.Errorf("expected stdout to be empty on fatal error, got %q", stdout)
	}

	if !strings.Contains(strings.ToLower(stderr), "target is required") {
		t.Errorf("expected fatal error about missing target on stderr, got: %q", stderr)
	}
}
