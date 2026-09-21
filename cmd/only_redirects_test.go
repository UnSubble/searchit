package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOnlyRedirectsMatrix(t *testing.T) {
	// 1. No follow:
	//    /login -> 302 /login/
	//    /login/ -> 200
	// Expected: [302] /login
	t.Run("1_NoFollow", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/login":
				http.Redirect(w, r, "/login/", http.StatusFound)
			case "/login/":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("login\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(outStr, "[302]") || !strings.Contains(outStr, "/login") {
			t.Errorf("expected '[302]' and '/login' in output, got:\n%s", outStr)
		}
		if strings.Contains(outStr, "[+] 200") || (strings.Contains(outStr, "/login/") && !strings.Contains(outStr, "->")) {
			t.Errorf("expected no final 200 result in no-follow mode, got:\n%s", outStr)
		}
	})

	// 2. Follow:
	//    /login -> 302 /login/
	//    /login/ -> 200
	// Expected exactly: [200] /login/
	t.Run("2_Follow", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/login":
				http.Redirect(w, r, "/login/", http.StatusFound)
			case "/login/":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("login\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--follow-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(outStr, "[+] 200") || !strings.Contains(outStr, "/login/") {
			t.Errorf("expected '[+] 200' and '/login/' in output, got:\n%s", outStr)
		}
		if strings.Contains(outStr, "[302]") {
			t.Errorf("expected intermediate [302] to NOT be in output, got:\n%s", outStr)
		}
	})

	// 3. Only redirects:
	//    same chain
	// Expected exactly: [200] /login/
	t.Run("3_OnlyRedirects", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/login":
				http.Redirect(w, r, "/login/", http.StatusFound)
			case "/login/":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
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

		if !strings.Contains(outStr, "[+] 200") || !strings.Contains(outStr, "/login/") {
			t.Errorf("expected '[+] 200' and '/login/' in output, got:\n%s", outStr)
		}
		if strings.Contains(outStr, "[302]") {
			t.Errorf("expected intermediate [302] to NOT be in output, got:\n%s", outStr)
		}
	})

	// 4. Direct 200 with only-redirects:
	//    /direct -> 200
	// Expected: no result
	t.Run("4_Direct200_WithOnlyRedirects", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/direct" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("direct"))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("direct\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if strings.Contains(outStr, "[+] 200") || strings.Contains(outStr, "/direct") {
			t.Errorf("expected no result for direct 200 in --only-redirects mode, got:\n%s", outStr)
		}
	})

	// 5. Redirect + final 404:
	//    /old -> 302 /missing
	//    /missing -> 404
	// Expected final result: [404] /missing
	t.Run("5_RedirectFinal404", func(t *testing.T) {
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

		// Test with --follow-redirects --mc 404
		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--follow-redirects", "--mc", "404",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(outStr, "[+] 404") || !strings.Contains(outStr, "/missing") {
			t.Errorf("expected '[+] 404' and '/missing' in output, got:\n%s", outStr)
		}
		if strings.Contains(outStr, "[302]") {
			t.Errorf("expected intermediate [302] to NOT be in output, got:\n%s", outStr)
		}

		// Also test with --only-redirects --mc 404
		outStr2, err2 := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "--mc", "404",
		})
		if err2 != nil {
			t.Fatalf("unexpected error with --only-redirects: %v", err2)
		}
		if !strings.Contains(outStr2, "[+] 404") || !strings.Contains(outStr2, "/missing") {
			t.Errorf("expected '[+] 404' and '/missing' with --only-redirects, got:\n%s", outStr2)
		}
	})

	// 6. Redirect + filter-code 200:
	//    /login -> 302 /login/
	//    /login/ -> 200
	// With: --filter-code 200
	// Expected: [200] /login/
	t.Run("6_Redirect_FilterCode200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/login":
				http.Redirect(w, r, "/login/", http.StatusFound)
			case "/login/":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("login\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--follow-redirects", "--mc", "200",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(outStr, "[+] 200") || !strings.Contains(outStr, "/login/") {
			t.Errorf("expected '[+] 200' and '/login/' with --mc 200, got:\n%s", outStr)
		}
		if strings.Contains(outStr, "[302]") {
			t.Errorf("expected intermediate [302] to NOT be in output, got:\n%s", outStr)
		}
	})

	// 7. Redirect + filter-code 302:
	//    same chain
	// With: --mc 302
	// Expected: no result
	t.Run("7_Redirect_FilterCode302", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/login":
				http.Redirect(w, r, "/login/", http.StatusFound)
			case "/login/":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("login\n"), 0644)

		outStr, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--follow-redirects", "--mc", "302",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if strings.Contains(outStr, "[+] 200") || strings.Contains(outStr, "[302]") || strings.Contains(outStr, "/login") {
			t.Errorf("expected no result when final response (200) does not match --mc 302, got:\n%s", outStr)
		}
	})

	// 8. Multi-hop chain:
	//    /a -> 301 /b
	//    /b -> 302 /c
	//    /c -> 200
	// Expected exactly one result: [200] /c
	t.Run("8_MultiHopChain", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/a":
				http.Redirect(w, r, "/b", http.StatusMovedPermanently)
			case "/b":
				http.Redirect(w, r, "/c", http.StatusFound)
			case "/c":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("final C"))
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

		if !strings.Contains(outStr, "[+] 200") || !strings.Contains(outStr, "/c") {
			t.Errorf("expected exactly '[+] 200' and '/c' in output, got:\n%s", outStr)
		}
		if strings.Contains(outStr, "[301]") || strings.Contains(outStr, "[302]") {
			t.Errorf("expected intermediate 301/302 to NOT be in output, got:\n%s", outStr)
		}

		count := strings.Count(outStr, "[+] 200")
		if count != 1 {
			t.Errorf("expected exactly 1 finding for multi-hop chain, got %d:\n%s", count, outStr)
		}
	})

	// 9. Flag conflict:
	//    --follow-redirects --only-redirects
	// Expected: validation error, zero requests
	t.Run("9_FlagConflict_FollowThenOnly", func(t *testing.T) {
		var reqCount int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&reqCount, 1)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("test\n"), 0644)

		_, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--follow-redirects", "--only-redirects",
		})
		if err == nil {
			t.Fatal("expected validation error for --follow-redirects and --only-redirects, got nil")
		}
		if !strings.Contains(err.Error(), "--follow-redirects cannot be used with --only-redirects") {
			t.Errorf("unexpected error message: %v", err)
		}
		if count := atomic.LoadInt64(&reqCount); count != 0 {
			t.Errorf("expected zero HTTP requests, got %d", count)
		}
	})

	// 10. Reverse flag order:
	//    --only-redirects --follow-redirects
	// Expected: validation error, zero requests
	t.Run("10_FlagConflict_OnlyThenFollow", func(t *testing.T) {
		var reqCount int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&reqCount, 1)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlPath := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlPath, []byte("test\n"), 0644)

		_, err := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "--follow-redirects",
		})
		if err == nil {
			t.Fatal("expected validation error for --only-redirects and --follow-redirects, got nil")
		}
		if !strings.Contains(err.Error(), "--follow-redirects cannot be used with --only-redirects") {
			t.Errorf("unexpected error message: %v", err)
		}
		if count := atomic.LoadInt64(&reqCount); count != 0 {
			t.Errorf("expected zero HTTP requests, got %d", count)
		}
	})

	// 11. Structured output:
	// Verify JSON/NDJSON/CSV contain only the final response and do not emit intermediate 3xx responses.
	t.Run("11_StructuredOutput", func(t *testing.T) {
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

		// A. JSON format with --only-redirects
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

		// B. NDJSON format with --only-redirects
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

		// C. CSV format with --only-redirects
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
		if strings.Contains(csvStr, "302") {
			t.Errorf("csv must not contain 302 intermediate response, got:\n%s", csvStr)
		}
	})

	// Loop & limits handling
	t.Run("RedirectLoopsAndLimits", func(t *testing.T) {
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

		outStr, _ := runIntegrationCommand([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "--max-redirects", "1",
		})

		if strings.Contains(outStr, "[+] 200") {
			t.Errorf("loop / limit exceeded requests should NOT produce 200 findings, got:\n%s", outStr)
		}
	})

	// Recursive scan with --only-redirects
	t.Run("RecursiveScan_OnlyRedirects", func(t *testing.T) {
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

	// CLI Help contains --only-redirects on both scan and fuzz, and does NOT contain --filter-code
	t.Run("CLIHelpVerification", func(t *testing.T) {
		scanCmd, _ := NewScanCmd()
		var sb strings.Builder
		scanCmd.SetOut(&sb)
		scanCmd.SetArgs([]string{"--help"})
		_ = scanCmd.Execute()

		helpStr := sb.String()
		if !strings.Contains(helpStr, "--only-redirects") {
			t.Errorf("expected '--only-redirects' in 'searchit scan --help', got:\n%s", helpStr)
		}
		if strings.Contains(helpStr, "--filter-code") {
			t.Errorf("expected '--filter-code' to NOT be in 'searchit scan --help', got:\n%s", helpStr)
		}

		fuzzCmd, _ := NewFuzzCmd()
		var fsb strings.Builder
		fuzzCmd.SetOut(&fsb)
		fuzzCmd.SetArgs([]string{"--help"})
		_ = fuzzCmd.Execute()

		fuzzHelpStr := fsb.String()
		if !strings.Contains(fuzzHelpStr, "--only-redirects") {
			t.Errorf("expected '--only-redirects' in 'searchit fuzz --help', got:\n%s", fuzzHelpStr)
		}
	})
}
