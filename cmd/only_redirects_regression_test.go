package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Test 1: Direct 200 without redirect is suppressed under --only-redirects (scan and fuzz)
func TestOnlyRedirectsRegression_Direct200Suppressed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("direct response"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.txt")
	_ = os.WriteFile(wlPath, []byte("item1\nitem2\n"), 0644)

	t.Run("Scan", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(stdout, "[+] 200") || strings.Contains(stdout, "/item1") || strings.Contains(stdout, "/item2") {
			t.Errorf("expected no results for direct 200 under --only-redirects, got:\n%s", stdout)
		}
	})

	t.Run("Fuzz", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(stdout, "[+] 200") || strings.Contains(stdout, "/item1") || strings.Contains(stdout, "/item2") {
			t.Errorf("expected no results for direct 200 under --only-redirects, got:\n%s", stdout)
		}
	})
}

// Test 2: 302 -> 200 redirect chain reports final 200 response, not 302 (scan and fuzz)
func TestOnlyRedirectsRegression_302To200_ReportsFinal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			http.Redirect(w, r, "/dashboard/", http.StatusFound)
		case "/dashboard/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("dashboard home"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.txt")
	_ = os.WriteFile(wlPath, []byte("login\n"), 0644)

	t.Run("Scan", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout, "[+] 200") || !strings.Contains(stdout, "/dashboard/") {
			t.Errorf("expected final [200] /dashboard/ in output, got:\n%s", stdout)
		}
		if strings.Contains(stdout, "[302]") {
			t.Errorf("intermediate [302] must not appear in output, got:\n%s", stdout)
		}
	})

	t.Run("Fuzz", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout, "[+] 200") || !strings.Contains(stdout, "/dashboard/") {
			t.Errorf("expected final [200] /dashboard/ in output, got:\n%s", stdout)
		}
		if strings.Contains(stdout, "[302]") {
			t.Errorf("intermediate [302] must not appear in output, got:\n%s", stdout)
		}
	})
}

// Test 3: 302 -> 404 final response evaluated against status matchers (scan and fuzz)
func TestOnlyRedirectsRegression_302To404_Filtering(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/old":
			http.Redirect(w, r, "/missing/", http.StatusFound)
		case "/missing/":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found here"))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.txt")
	_ = os.WriteFile(wlPath, []byte("old\n"), 0644)

	t.Run("Scan_MatchedWithMC404", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "--mc", "404",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout, "[+] 404") || !strings.Contains(stdout, "/missing/") {
			t.Errorf("expected final [404] /missing/ in output when matched via --mc 404, got:\n%s", stdout)
		}
	})

	t.Run("Scan_SuppressedByDefault404Exclude", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(stdout, "[+] 404") || strings.Contains(stdout, "/missing/") {
			t.Errorf("expected final 404 to be suppressed by default status filter, got:\n%s", stdout)
		}
	})

	t.Run("Fuzz_MatchedWithMC404", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects", "--mc", "404",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout, "[+] 404") || !strings.Contains(stdout, "/missing/") {
			t.Errorf("expected final [404] /missing/ in output when matched via --mc 404, got:\n%s", stdout)
		}
	})

	t.Run("Fuzz_SuppressedByDefault404Exclude", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(stdout, "[+] 404") || strings.Contains(stdout, "/missing/") {
			t.Errorf("expected final 404 to be suppressed by default status filter, got:\n%s", stdout)
		}
	})
}

// Test 4: 301 -> 302 -> 200 multi-hop chain produces exactly one logical result (scan and fuzz)
func TestOnlyRedirectsRegression_MultiHopChain_SingleResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/step1", http.StatusMovedPermanently)
		case "/step1":
			http.Redirect(w, r, "/step2", http.StatusFound)
		case "/step2":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("final content"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.txt")
	_ = os.WriteFile(wlPath, []byte("start\n"), 0644)

	t.Run("Scan", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout, "[+] 200") || !strings.Contains(stdout, "/step2") {
			t.Errorf("expected final [200] /step2, got:\n%s", stdout)
		}
		if strings.Contains(stdout, "301") || strings.Contains(stdout, "302") {
			t.Errorf("intermediate redirect codes must not appear in output, got:\n%s", stdout)
		}
		// Must be exactly one finding line
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		findingCount := 0
		for _, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "[+]") {
				findingCount++
			}
		}
		if findingCount != 1 {
			t.Errorf("expected exactly 1 finding line, got %d:\n%s", findingCount, stdout)
		}
	})

	t.Run("Fuzz", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout, "[+] 200") || !strings.Contains(stdout, "/step2") {
			t.Errorf("expected final [200] /step2, got:\n%s", stdout)
		}
		if strings.Contains(stdout, "301") || strings.Contains(stdout, "302") {
			t.Errorf("intermediate redirect codes must not appear in output, got:\n%s", stdout)
		}
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		findingCount := 0
		for _, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "[+]") {
				findingCount++
			}
		}
		if findingCount != 1 {
			t.Errorf("expected exactly 1 finding line, got %d:\n%s", findingCount, stdout)
		}
	})
}

// Test 5: --mc and --fc evaluate final response status code, not intermediate 3xx (scan and fuzz)
func TestOnlyRedirectsRegression_FilterCodeFinalResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/entry":
			http.Redirect(w, r, "/forbidden", http.StatusFound)
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("forbidden access"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.txt")
	_ = os.WriteFile(wlPath, []byte("entry\n"), 0644)

	t.Run("Scan_MC403_MatchesFinal", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "--mc", "403",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout, "[+] 403") || !strings.Contains(stdout, "/forbidden") {
			t.Errorf("expected [403] /forbidden to match --mc 403, got:\n%s", stdout)
		}
	})

	t.Run("Scan_FC403_SuppressesFinal", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "--fc", "403",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(stdout, "[+] 403") || strings.Contains(stdout, "/forbidden") {
			t.Errorf("expected [403] /forbidden to be filtered out by --fc 403, got:\n%s", stdout)
		}
	})

	t.Run("Scan_MC302_DoesNotMatchFinal403", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "--mc", "302",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(stdout, "/forbidden") || strings.Contains(stdout, "/entry") {
			t.Errorf("expected no result when final status is 403 and filter is --mc 302, got:\n%s", stdout)
		}
	})

	t.Run("Fuzz_MC403_MatchesFinal", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects", "--mc", "403",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout, "[+] 403") || !strings.Contains(stdout, "/forbidden") {
			t.Errorf("expected [403] /forbidden to match --mc 403, got:\n%s", stdout)
		}
	})

	t.Run("Fuzz_FC403_SuppressesFinal", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects", "--fc", "403",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(stdout, "[+] 403") || strings.Contains(stdout, "/forbidden") {
			t.Errorf("expected [403] /forbidden to be filtered out by --fc 403, got:\n%s", stdout)
		}
	})

	t.Run("Fuzz_MC302_DoesNotMatchFinal403", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects", "--mc", "302",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(stdout, "/forbidden") || strings.Contains(stdout, "/entry") {
			t.Errorf("expected no result when final status is 403 and filter is --mc 302, got:\n%s", stdout)
		}
	})
}

// Test 6 & 7: Candidate accounting is completely independent of --only-redirects (100 candidates)
func TestOnlyRedirectsRegression_CandidateAccounting(t *testing.T) {
	totalWords := 100
	redirectCount := 10

	var words []string
	for i := 0; i < totalWords; i++ {
		words = append(words, fmt.Sprintf("word%03d", i))
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		for i := 0; i < redirectCount; i++ {
			targetWord := fmt.Sprintf("word%03d", i)
			if path == targetWord {
				http.Redirect(w, r, "/landing", http.StatusFound)
				return
			}
		}
		if path == "landing" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("landing page"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("regular direct page"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.txt")
	_ = os.WriteFile(wlPath, []byte(strings.Join(words, "\n")+"\n"), 0644)

	t.Run("Scan_CandidatesSummary", func(t *testing.T) {
		stdout, stderr, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		combined := stdout + "\n" + stderr
		if !strings.Contains(combined, "Candidates") || !strings.Contains(combined, "100") {
			t.Errorf("expected summary Candidates to be 100, got:\n%s", combined)
		}
		if !strings.Contains(combined, "Findings") || !strings.Contains(combined, "10") {
			t.Errorf("expected summary Findings to be 10, got:\n%s", combined)
		}
	})

	t.Run("Fuzz_CandidatesSummary", func(t *testing.T) {
		stdout, stderr, err := runIntegrationCommandStreams([]string{
			"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		combined := stdout + "\n" + stderr
		if !strings.Contains(combined, "Candidates") || !strings.Contains(combined, "100") {
			t.Errorf("expected summary Candidates to be 100, got:\n%s", combined)
		}
		if !strings.Contains(combined, "Findings") || !strings.Contains(combined, "10") {
			t.Errorf("expected summary Findings to be 10, got:\n%s", combined)
		}
	})
}

// Test 8: Structured output formats (JSON, NDJSON, CSV) emit only final response (status 200, final URL)
func TestOnlyRedirectsRegression_StructuredOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			http.Redirect(w, r, "/dashboard", http.StatusFound)
		case "/dashboard":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("dashboard page"))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("direct page"))
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.txt")
	_ = os.WriteFile(wlPath, []byte("login\ndirect\n"), 0644)

	t.Run("JSON_Fuzz", func(t *testing.T) {
		outPath := filepath.Join(tmpDir, "out.json")
		_, _, err := runIntegrationCommandStreams([]string{
			"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects", "-o", outPath, "--format", "json",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("failed reading output: %v", err)
		}
		var records []struct {
			URL    string `json:"url"`
			Status int    `json:"status"`
		}
		if err := json.Unmarshal(data, &records); err != nil {
			t.Fatalf("failed unmarshaling json: %v, raw data:\n%s", err, string(data))
		}
		if len(records) != 1 {
			t.Fatalf("expected exactly 1 record, got %d: %+v", len(records), records)
		}
		if records[0].Status != 200 {
			t.Errorf("expected status 200, got %d", records[0].Status)
		}
		if !strings.HasSuffix(records[0].URL, "/dashboard") {
			t.Errorf("expected final URL ending in /dashboard, got %s", records[0].URL)
		}
	})

	t.Run("NDJSON_Fuzz", func(t *testing.T) {
		outPath := filepath.Join(tmpDir, "out.ndjson")
		_, _, err := runIntegrationCommandStreams([]string{
			"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects", "-o", outPath, "--format", "ndjson",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("failed reading output: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) != 1 {
			t.Fatalf("expected exactly 1 ndjson line, got %d:\n%s", len(lines), string(data))
		}
		var rec struct {
			URL    string `json:"url"`
			Status int    `json:"status"`
		}
		if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
			t.Fatalf("failed unmarshaling line: %v", err)
		}
		if rec.Status != 200 {
			t.Errorf("expected status 200, got %d", rec.Status)
		}
		if !strings.HasSuffix(rec.URL, "/dashboard") {
			t.Errorf("expected final URL ending in /dashboard, got %s", rec.URL)
		}
	})

	t.Run("CSV_Scan", func(t *testing.T) {
		outPath := filepath.Join(tmpDir, "out.csv")
		_, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "-o", outPath, "--format", "csv",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("failed reading output: %v", err)
		}
		r := csv.NewReader(strings.NewReader(string(data)))
		records, err := r.ReadAll()
		if err != nil {
			t.Fatalf("failed parsing csv: %v", err)
		}
		// Header + 1 row
		if len(records) != 2 {
			t.Fatalf("expected 2 csv rows (header + 1 result), got %d: %+v", len(records), records)
		}
		row := records[1]
		// row: [url, status, length, depth]
		if !strings.HasSuffix(row[0], "/dashboard") {
			t.Errorf("expected csv url ending in /dashboard, got %s", row[0])
		}
		if row[1] != "200" {
			t.Errorf("expected csv status '200', got %s", row[1])
		}
	})
}

// Test 9: --only-redirects implicitly follows redirects in HTTP client
func TestOnlyRedirectsRegression_ImplicitlyFollows(t *testing.T) {
	var requestedPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.Path)
		if r.URL.Path == "/redir" {
			http.Redirect(w, r, "/destination", http.StatusFound)
			return
		}
		if r.URL.Path == "/destination" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("dest"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.txt")
	_ = os.WriteFile(wlPath, []byte("redir\n"), 0644)

	stdout, _, err := runIntegrationCommandStreams([]string{
		"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "/destination") {
		t.Errorf("expected /destination in output (indicating redirect was followed), got:\n%s", stdout)
	}
	// Check that both /redir and /destination were requested
	foundDest := false
	for _, p := range requestedPaths {
		if p == "/destination" {
			foundDest = true
			break
		}
	}
	if !foundDest {
		t.Errorf("expected server to receive request for /destination, paths received: %v", requestedPaths)
	}
}

// Test 10: --follow-redirects and --only-redirects mutual exclusion on CLI and options
func TestOnlyRedirectsRegression_MutualExclusion(t *testing.T) {
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.txt")
	_ = os.WriteFile(wlPath, []byte("word\n"), 0644)

	tests := []struct {
		name string
		args []string
	}{
		{"Scan_Follow_Then_Only", []string{"scan", "-u", srv.URL, "-w", wlPath, "--follow-redirects", "--only-redirects"}},
		{"Scan_Only_Then_Follow", []string{"scan", "-u", srv.URL, "-w", wlPath, "--only-redirects", "--follow-redirects"}},
		{"Fuzz_Follow_Then_Only", []string{"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--follow-redirects", "--only-redirects"}},
		{"Fuzz_Only_Then_Follow", []string{"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "--only-redirects", "--follow-redirects"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reqCount.Store(0)
			_, _, err := runIntegrationCommandStreams(tc.args)
			if err == nil {
				t.Fatalf("expected error for conflicting flags, got nil")
			}
			if !strings.Contains(err.Error(), "--follow-redirects cannot be used with --only-redirects") {
				t.Errorf("expected mutual exclusion error message, got: %v", err)
			}
			if reqCount.Load() != 0 {
				t.Errorf("expected 0 requests sent before validation failure, got %d", reqCount.Load())
			}
		})
	}
}
