package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlyRedirects(t *testing.T) {
	// A. Basic redirect:
	//    /login -> 302 /login/
	//    /login/ -> 200
	// Expected: [200] http://.../login/ and NOT [302] http://.../login
	t.Run("A_BasicRedirect", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/login":
				http.Redirect(w, r, "/login/", http.StatusFound)
			case "/login/":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("welcome"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("login\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(outStr, "[+] 200") {
			t.Errorf("expected '[+] 200' in output, got:\n%s", outStr)
		}
		if !strings.Contains(outStr, "/login/") {
			t.Errorf("expected final URL '/login/' in output, got:\n%s", outStr)
		}
		if strings.Contains(outStr, "[302]") {
			t.Errorf("expected intermediate [302] to NOT be in output, got:\n%s", outStr)
		}
	})

	// B. No redirect:
	//    /admin -> 200
	// Expected: no finding
	t.Run("B_NoRedirect_Suppressed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/admin" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("admin panel"))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("admin\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if strings.Contains(outStr, "[+] 200") || strings.Contains(outStr, "/admin") {
			t.Errorf("expected no finding for direct 200 in --only-redirects mode, got:\n%s", outStr)
		}
	})

	// C. Redirect to 404:
	//    /old -> 302 /missing
	//    /missing -> 404
	// Expected: [404] http://.../missing
	t.Run("C_RedirectTo404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/old":
				http.Redirect(w, r, "/missing", http.StatusFound)
			case "/missing":
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("not found"))
			default:
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("old\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "--mc", "404", "--fc", "500",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(outStr, "[+] 404") {
			t.Errorf("expected '[+] 404' in output, got:\n%s", outStr)
		}
		if !strings.Contains(outStr, "/missing") {
			t.Errorf("expected final URL '/missing' in output, got:\n%s", outStr)
		}
		if strings.Contains(outStr, "[302]") {
			t.Errorf("expected intermediate [302] to NOT be in output, got:\n%s", outStr)
		}
	})

	// D. Redirect chain:
	//    /a -> 301 /b
	//    /b -> 302 /c
	//    /c -> 200
	// Expected: [200] http://.../c
	t.Run("D_RedirectChain", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/a":
				http.Redirect(w, r, "/b", http.StatusMovedPermanently)
			case "/b":
				http.Redirect(w, r, "/c", http.StatusFound)
			case "/c":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("target C"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("a\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(outStr, "[+] 200") {
			t.Errorf("expected '[+] 200' in output, got:\n%s", outStr)
		}
		if !strings.Contains(outStr, "/c") {
			t.Errorf("expected final URL '/c' in output, got:\n%s", outStr)
		}
		if strings.Contains(outStr, "[301]") || strings.Contains(outStr, "[302]") {
			t.Errorf("expected intermediate 301/302 to NOT be in output, got:\n%s", outStr)
		}
	})

	// E. --only-redirects alone follows redirects
	t.Run("E_OnlyRedirectsAloneFollowsRedirects", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/redir":
				http.Redirect(w, r, "/final-dest", http.StatusTemporaryRedirect)
			case "/final-dest":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("redir\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(outStr, "[+] 200") || !strings.Contains(outStr, "/final-dest") {
			t.Errorf("expected --only-redirects alone to follow redirect to /final-dest, got:\n%s", outStr)
		}
	})

	// F. --only-redirects --follow-redirects behaves identically
	t.Run("F_CompatibleWithFollowRedirects", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/redir":
				http.Redirect(w, r, "/final-dest", http.StatusFound)
			case "/final-dest":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("redir\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "--follow-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(outStr, "[+] 200") || !strings.Contains(outStr, "/final-dest") {
			t.Errorf("expected --only-redirects with --follow-redirects to report final destination, got:\n%s", outStr)
		}
		if strings.Contains(outStr, "[302]") {
			t.Errorf("expected intermediate [302] to NOT be in output, got:\n%s", outStr)
		}
	})

	// G. Direct 200 and redirected 200 are distinguished
	t.Run("G_DirectVsRedirectedDistinction", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/direct":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("direct response"))
			case "/indirect":
				http.Redirect(w, r, "/target", http.StatusFound)
			case "/target":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("indirect response"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("direct\nindirect\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if strings.Contains(outStr, "/direct") {
			t.Errorf("direct 200 must NOT be emitted in --only-redirects mode, got:\n%s", outStr)
		}
		if !strings.Contains(outStr, "/target") {
			t.Errorf("redirected 200 /target must be emitted in --only-redirects mode, got:\n%s", outStr)
		}
	})

	// H. Structured output reports the final response and final URL
	t.Run("H_StructuredOutputFormats", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/login":
				http.Redirect(w, r, "/login/", http.StatusFound)
			case "/login/":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("hello world"))
			case "/direct":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("direct"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("direct\nlogin\n"), 0644)

		// 1. JSON format
		jsonFile := filepath.Join(tmpDir, "out.json")
		_, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "-o", jsonFile, "--format", "json",
		})
		if err != nil {
			t.Fatalf("json run failed: %v", err)
		}
		jsonBytes, err := os.ReadFile(jsonFile)
		if err != nil {
			t.Fatalf("failed to read json: %v", err)
		}
		var jsonResults []struct {
			URL    string `json:"url"`
			Status int    `json:"status"`
		}
		if err := json.Unmarshal(jsonBytes, &jsonResults); err != nil {
			t.Fatalf("invalid json: %v, content: %s", err, string(jsonBytes))
		}
		if len(jsonResults) != 1 {
			t.Fatalf("expected exactly 1 json result (only redirected), got %d: %+v", len(jsonResults), jsonResults)
		}
		if jsonResults[0].Status != 200 {
			t.Errorf("json status = %d, want 200", jsonResults[0].Status)
		}
		if !strings.HasSuffix(jsonResults[0].URL, "/login/") {
			t.Errorf("json URL = %q, want suffix '/login/'", jsonResults[0].URL)
		}

		// 2. NDJSON format
		ndjsonFile := filepath.Join(tmpDir, "out.ndjson")
		_, err = runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "-o", ndjsonFile, "--format", "ndjson",
		})
		if err != nil {
			t.Fatalf("ndjson run failed: %v", err)
		}
		ndjsonBytes, err := os.ReadFile(ndjsonFile)
		if err != nil {
			t.Fatalf("failed to read ndjson: %v", err)
		}
		ndLines := strings.Split(strings.TrimSpace(string(ndjsonBytes)), "\n")
		if len(ndLines) != 1 {
			t.Fatalf("expected 1 ndjson line, got %d: %q", len(ndLines), string(ndjsonBytes))
		}
		var ndResult struct {
			URL    string `json:"url"`
			Status int    `json:"status"`
		}
		if err := json.Unmarshal([]byte(ndLines[0]), &ndResult); err != nil {
			t.Fatalf("invalid ndjson: %v", err)
		}
		if ndResult.Status != 200 || !strings.HasSuffix(ndResult.URL, "/login/") {
			t.Errorf("ndjson result = %+v, want status 200 and URL suffix '/login/'", ndResult)
		}

		// 3. CSV format
		csvFile := filepath.Join(tmpDir, "out.csv")
		_, err = runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "-o", csvFile, "--format", "csv",
		})
		if err != nil {
			t.Fatalf("csv run failed: %v", err)
		}
		csvBytes, err := os.ReadFile(csvFile)
		if err != nil {
			t.Fatalf("failed to read csv: %v", err)
		}
		csvStr := string(csvBytes)
		if !strings.Contains(csvStr, "/login/,200") {
			t.Errorf("csv missing '/login/,200', got:\n%s", csvStr)
		}
		if strings.Contains(csvStr, "/direct") {
			t.Errorf("csv must not contain '/direct', got:\n%s", csvStr)
		}

		// 4. Quiet text format
		quietOut, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "-q",
		})
		if err != nil {
			t.Fatalf("quiet run failed: %v", err)
		}
		quietLines := strings.Fields(quietOut)
		if len(quietLines) != 1 || !strings.HasSuffix(quietLines[0], "/login/") {
			t.Errorf("quiet output = %q, want single line with suffix '/login/'", quietOut)
		}
	})

	// I. Existing --follow-redirects behavior remains unchanged
	t.Run("I_ExistingFollowRedirectsUnchanged", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/test":
				http.Redirect(w, r, "/target/", http.StatusMovedPermanently)
			case "/target/":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("hello"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("test\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--follow-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Existing behavior reports [301] with arrow -> destination
		if !strings.Contains(outStr, "[301]") {
			t.Errorf("expected '[301]' in normal --follow-redirects output, got:\n%s", outStr)
		}
		if !strings.Contains(outStr, "->") {
			t.Errorf("expected '->' in normal --follow-redirects output, got:\n%s", outStr)
		}
	})

	// J. Redirect loops / maximum redirect handling does not panic or emit invalid findings
	t.Run("J_RedirectLoopsAndLimits", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/loop1":
				http.Redirect(w, r, "/loop2", http.StatusFound)
			case "/loop2":
				http.Redirect(w, r, "/loop1", http.StatusFound)
			case "/chain1":
				http.Redirect(w, r, "/chain2", http.StatusFound)
			case "/chain2":
				http.Redirect(w, r, "/chain3", http.StatusFound)
			case "/chain3":
				http.Redirect(w, r, "/final", http.StatusFound)
			case "/final":
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("loop1\nchain1\n"), 0644)

		// With max-redirects 1, chain1 will exceed limit. loop1 will detect loop.
		outStr, _ := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "--max-redirects", "1",
		})

		// Neither should produce an accepted finding
		if strings.Contains(outStr, "[+] 200") {
			t.Errorf("loop / limit exceeded requests should NOT produce 200 findings, got:\n%s", outStr)
		}
	})

	// K. Recursive scan with --only-redirects
	t.Run("K_RecursiveScan", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/":
				w.WriteHeader(http.StatusOK)
			case "/redir":
				http.Redirect(w, r, "/dest/", http.StatusFound)
			case "/dest/":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("in dest"))
			case "/direct":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("in direct"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("direct\nredir\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "-r", "--max-depth", "1", "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if strings.Contains(outStr, "/direct") {
			t.Errorf("direct response should not be emitted in --only-redirects recursive mode, got:\n%s", outStr)
		}
		if !strings.Contains(outStr, "/dest/") {
			t.Errorf("redirected response /dest/ should be emitted in --only-redirects recursive mode, got:\n%s", outStr)
		}
	})

	// L. CLI Help contains --only-redirects
	t.Run("L_CLIHelpVerification", func(t *testing.T) {
		scanCmd, _ := NewScanCmd()
		var sb strings.Builder
		scanCmd.SetOut(&sb)
		scanCmd.SetArgs([]string{"--help"})
		_ = scanCmd.Execute()

		helpStr := sb.String()
		if !strings.Contains(helpStr, "--only-redirects") {
			t.Errorf("expected '--only-redirects' in 'searchit scan --help', got:\n%s", helpStr)
		}
	})
}
