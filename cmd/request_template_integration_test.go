package cmd

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCLI_ScanWithRequestTemplate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %q", r.Method)
		}
		if r.Host != "scan-test.local" {
			t.Errorf("expected Host scan-test.local, got %q", r.Host)
		}
		if r.Header.Get("X-Scan-Header") != "ScanValue" {
			t.Errorf("expected X-Scan-Header=ScanValue, got %q", r.Header.Get("X-Scan-Header"))
		}
		cookie, _ := r.Cookie("session")
		if cookie == nil || cookie.Value != "active" {
			t.Errorf("expected cookie session=active")
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	// Create temporary request template file
	tmplContent := "POST /endpoint HTTP/1.1\nHost: scan-test.local\nX-Scan-Header: ScanValue\nCookie: session=active\n\nmy-post-body"
	tmpFile, err := os.CreateTemp("", "request_scan_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.WriteString(tmplContent)
	tmpFile.Close()

	// Create temp wordlist
	tmpWordlist, err := os.CreateTemp("", "words_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp wordlist: %v", err)
	}
	defer os.Remove(tmpWordlist.Name())
	_, _ = tmpWordlist.WriteString("dummy\n")
	tmpWordlist.Close()

	// Redirect stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Reset flags and setup rootCmd
	resetFlagsForTest()
	defer resetFlagsForTest()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	args := []string{
		"scan",
		"-u", srv.URL,
		"--request", tmpFile.Name(),
		"-w", tmpWordlist.Name(),
		"--mc", "200",
		"--no-progress",
	}
	rootCmd.SetArgs(args)

	err = rootCmd.ExecuteContext(context.Background())
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("execute failed: %v. Output:\n%s", err, buf.String())
	}

	var stdoutBuf bytes.Buffer
	if _, err := io.Copy(&stdoutBuf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	out := stdoutBuf.String()

	if !strings.Contains(out, "200") {
		t.Errorf("expected 200 status code in output, got:\n%s", out)
	}
}

func TestCLI_FuzzWithRequestTemplate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/myfuzzval" {
			t.Errorf("expected path /api/myfuzzval, got %q", r.URL.Path)
		}
		if r.Header.Get("X-Fuzz-Header") != "FuzzValue" {
			t.Errorf("expected X-Fuzz-Header=FuzzValue, got %q", r.Header.Get("X-Fuzz-Header"))
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fuzz-response"))
	}))
	t.Cleanup(srv.Close)

	// Create temporary request template file with FUZZ placeholder in path
	tmplContent := "GET /api/FUZZ HTTP/1.1\nHost: fuzz-test.local\nX-Fuzz-Header: FuzzValue\n\n"
	tmpFile, err := os.CreateTemp("", "request_fuzz_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.WriteString(tmplContent)
	tmpFile.Close()

	// Create temp wordlist
	tmpWordlist, err := os.CreateTemp("", "words_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp wordlist: %v", err)
	}
	defer os.Remove(tmpWordlist.Name())
	_, _ = tmpWordlist.WriteString("myfuzzval\n")
	tmpWordlist.Close()

	// Redirect stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Reset flags and setup rootCmd
	resetFlagsForTest()
	defer resetFlagsForTest()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	args := []string{
		"fuzz",
		"-u", srv.URL,
		"--request", tmpFile.Name(),
		"-w", tmpWordlist.Name(),
		"--mc", "200",
	}
	rootCmd.SetArgs(args)

	err = rootCmd.ExecuteContext(context.Background())
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("execute failed: %v. Output:\n%s", err, buf.String())
	}

	var stdoutBuf bytes.Buffer
	if _, err := io.Copy(&stdoutBuf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	out := stdoutBuf.String()

	if !strings.Contains(out, "myfuzzval") {
		t.Errorf("expected 'myfuzzval' in output, got:\n%s", out)
	}
}
