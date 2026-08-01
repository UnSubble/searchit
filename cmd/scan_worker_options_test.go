package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScan_WorkerOptions_LinkExtractionDifference(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.php" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><body><a href="flag_1.txt">Flag 1</a></body></html>`))
			return
		}
		if r.URL.Path == "/flag_1.txt" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`FLAG{test_flag_1}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.txt")
	if err := os.WriteFile(wlPath, []byte("index.php\n"), 0600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	t.Run("RecursiveScan_DiscoversHTMLLinkedResource", func(t *testing.T) {
		out, err := runIntegrationCommand([]string{"scan", "-u", srv.URL, "-w", wlPath, "-r", "-d", "2", "-s", "priority", "--mc", "200", "-q"})
		if err != nil {
			t.Fatalf("unexpected error executing recursive scan: %v", err)
		}

		if !strings.Contains(out, "flag_1.txt") {
			t.Errorf("expected recursive scan to discover flag_1.txt via HTML link extraction, got output:\n%s", out)
		}
	})

	t.Run("StandardScan_DoesNotDiscoverHTMLLinkedResource", func(t *testing.T) {
		out, err := runIntegrationCommand([]string{"scan", "-u", srv.URL, "-w", wlPath, "--mc", "200", "-q"})
		if err != nil {
			t.Fatalf("unexpected error executing standard scan: %v", err)
		}

		if strings.Contains(out, "flag_1.txt") {
			t.Errorf("expected standard scan NOT to discover flag_1.txt, got output:\n%s", out)
		}
	})
}
