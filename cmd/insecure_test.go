package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/httpclient"
)

func TestInsecureFlag_Cases(t *testing.T) {
	// Create an untrusted HTTPS server (self-signed / unverified cert)
	untrustedHTTPS := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/admin" || r.URL.Path == "/login" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("untrusted https content"))
			return
		}
		http.NotFound(w, r)
	}))
	defer untrustedHTTPS.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	os.WriteFile(wlPath, []byte("admin\nlogin\nsecret\n"), 0644)

	t.Run("Case 1 & 2: HTTPClient without insecure parameter rejects untrusted TLS certs", func(t *testing.T) {
		client := httpclient.NewWithHTTPVersion(5*time.Second, 2*time.Second, false, 10, "", "auto", false)
		req, _ := http.NewRequestWithContext(context.Background(), "GET", untrustedHTTPS.URL, nil)
		_, err := client.Do(req)
		if err == nil {
			t.Fatalf("expected TLS certificate verification error without insecure flag, got nil")
		}
	})

	t.Run("Case 3: HTTPClient with insecure parameter accepts untrusted TLS certs", func(t *testing.T) {
		client := httpclient.NewWithHTTPVersion(5*time.Second, 2*time.Second, false, 10, "", "auto", true)
		req, _ := http.NewRequestWithContext(context.Background(), "GET", untrustedHTTPS.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("unexpected error with insecure=true: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}
	})

	t.Run("CLI scan flag binding -k maps to cfg.Insecure = true", func(t *testing.T) {
		scanCmd, opts := NewScanCmd()
		if err := scanCmd.ParseFlags([]string{"-u", "https://example.com", "-w", wlPath, "-k"}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}
		cfg := config.Default()
		applyCLIOverrides(opts, scanCmd, &cfg)
		if !cfg.Insecure {
			t.Errorf("expected cfg.Insecure = true when -k is passed, got false")
		}

		application := app.New(context.Background(), cfg)
		if application.HTTPClient == nil {
			t.Fatalf("expected HTTPClient to be initialized")
		}
	})

	t.Run("CLI scan flag binding --insecure maps to cfg.Insecure = true", func(t *testing.T) {
		scanCmd, opts := NewScanCmd()
		if err := scanCmd.ParseFlags([]string{"-u", "https://example.com", "-w", wlPath, "--insecure"}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}
		cfg := config.Default()
		applyCLIOverrides(opts, scanCmd, &cfg)
		if !cfg.Insecure {
			t.Errorf("expected cfg.Insecure = true when --insecure is passed, got false")
		}
	})

	t.Run("CLI scan default without -k maps to cfg.Insecure = false", func(t *testing.T) {
		scanCmd, opts := NewScanCmd()
		if err := scanCmd.ParseFlags([]string{"-u", "https://example.com", "-w", wlPath}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}
		cfg := config.Default()
		applyCLIOverrides(opts, scanCmd, &cfg)
		if cfg.Insecure {
			t.Errorf("expected cfg.Insecure = false by default, got true")
		}
	})

	t.Run("CLI fuzz flag binding -k maps to cfg.Insecure = true", func(t *testing.T) {
		fuzzCmd, opts := NewFuzzCmd()
		if err := fuzzCmd.ParseFlags([]string{"-u", "https://example.com/FUZZ", "-w", wlPath, "-k"}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}
		cfg := config.Default()
		applyFuzzCLIOverrides(opts, fuzzCmd, &cfg)
		if !cfg.Insecure {
			t.Errorf("expected cfg.Insecure = true when fuzz -k is passed, got false")
		}
	})
}
