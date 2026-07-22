package cmd

import (
	"fmt"
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

	out, err := runIntegrationCommand([]string{
		"scan",
		"-u", srv.URL,
		"--request", tmpFile.Name(),
		"-w", tmpWordlist.Name(),
		"--mc", "200",
		"--no-progress",
	})

	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

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
	tmplContent := fmt.Sprintf("GET /api/FUZZ HTTP/1.1\nHost: %s\nX-Fuzz-Header: FuzzValue\n\n", strings.TrimPrefix(srv.URL, "http://"))
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

	out, err := runIntegrationCommand([]string{
		"fuzz",
		"--request", tmpFile.Name(),
		"-w", tmpWordlist.Name(),
		"--mc", "200",
	})

	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if !strings.Contains(out, "myfuzzval") {
		t.Errorf("expected 'myfuzzval' in output, got:\n%s", out)
	}
}
