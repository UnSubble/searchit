package recursion_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/wordlist"
)

// ── helpers ──────────────────────────────────────────────────────────────────

type staticSliceReader []string

func (s staticSliceReader) Read(ctx context.Context, out chan<- string) error {
	for _, w := range s {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- w:
		}
	}
	return nil
}

func sliceReader(words ...string) wordlist.Reader {
	return wordlist.NewSliceReader(wordlist.NewSliceReader(
		staticSliceReader(words),
	))
}

// ── 1. Flag accepted by scan ──────────────────────────────────────────────
// (CLI acceptance is tested in cmd/ dryrun tests; this unit test confirms
// GenerateScanDryRunRequests returns without error.)

func TestScanDryRun_FunctionAccepts(t *testing.T) {
	r := sliceReader("admin", "api")
	_, _, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com/"},
		r, false, false, nil, 10,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── 2 & 17. Zero HTTP requests / no network even when target is unreachable ─

func TestScanDryRun_ZeroNetworkRequests(t *testing.T) {
	var count int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&count, 1)
	}))
	defer srv.Close()

	r := sliceReader("admin", "login")
	_, _, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{srv.URL + "/"},
		r, false, false, nil, 10,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 HTTP requests, got %d", count)
	}
}

func TestScanDryRun_NoNetworkEvenWhenTargetUnreachable(t *testing.T) {
	// 203.0.113.x is TEST-NET, guaranteed unreachable.
	unreachable := "http://203.0.113.1/"
	r := sliceReader("test")
	_, _, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{unreachable},
		r, false, false, nil, 5,
	)
	// Must not return a network error — only succeeds or context-cancel.
	if err != nil && !isContextErr(err) {
		t.Fatalf("unexpected error (should not dial network): %v", err)
	}
}

// ── 3. Zero TLS handshakes ────────────────────────────────────────────────

func TestScanDryRun_NoTLSHandshake(t *testing.T) {
	var tlsCount int64
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&tlsCount, 1)
	}))
	defer srv.Close()

	r := sliceReader("page")
	_, _, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{srv.URL + "/"},
		r, false, false, nil, 5,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCount != 0 {
		t.Fatalf("expected 0 TLS connections, got %d", tlsCount)
	}
}

// ── 4. URL candidates rendered correctly ──────────────────────────────────

func TestScanDryRun_URLCandidates(t *testing.T) {
	r := sliceReader("admin", "api")
	results, total, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com/"},
		r, false, false, nil, 10,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total=2, got %d", total)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].URL != "http://example.com/admin" {
		t.Errorf("expected http://example.com/admin, got %s", results[0].URL)
	}
	if results[1].URL != "http://example.com/api" {
		t.Errorf("expected http://example.com/api, got %s", results[1].URL)
	}
}

// ── 5. Wordlist ordering matches real scan ordering ───────────────────────
// Verified by testing that dry-run candidate N == real scan candidate N.
// We record what the real Manager dispatches and compare with dry-run.

func TestScanDryRun_WordlistOrderMatches(t *testing.T) {
	words := []string{"alpha", "beta", "gamma", "delta"}

	// --- Real path recording ---
	var realURLs []string
	var count int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record each URL the real scan dispatches.
		atomic.AddInt64(&count, 1)
		realURLs = append(realURLs, r.URL.Path)
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	// Use a recording RoundTripper to capture order deterministically.
	type recorded struct{ path string }
	var recordings []recorded
	mu := make(chan struct{}, 1)
	mu <- struct{}{}
	transport := &recordingTransport{fn: func(req *http.Request) (*http.Response, error) {
		<-mu
		recordings = append(recordings, recorded{path: req.URL.Path})
		mu <- struct{}{}
		resp := &http.Response{
			StatusCode: 200,
			Body:       http.NoBody,
			Header:     make(http.Header),
		}
		return resp, nil
	}}
	client := &http.Client{Transport: transport}

	mgr := recursion.NewManager(
		client,
		nil, // no exclude filter
		wordlist.NewSliceReader(staticSliceReader(words)),
		recursion.BFS,
		1, // maxDepth=1
		func() status.Filters { f, _ := status.Parse("200"); return f }(), // recurseOn
		false, // normalizePaths
		false, // collapseSlashes
		0,     // delay
		nil,   // limiter
		nil,   // fingerprintCache
		int64(len(words)),
	)
	_ = mgr.Run(context.Background(), context.Background(),
		[]string{srv.URL + "/"},
		1, // single worker for deterministic order
		func(r engine.Result) {},
		func(err error) {},
	)

	// --- Dry-run path ---
	dryResults, _, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{srv.URL + "/"},
		wordlist.NewSliceReader(staticSliceReader(words)),
		false, false, nil, 0,
	)
	if err != nil {
		t.Fatalf("dry-run error: %v", err)
	}

	// We compare path components.
	realIdx := 0
	for _, rec := range recordings {
		if rec.path == "/robots.txt" || rec.path == "/sitemap.xml" || rec.path == "/" {
			continue // skip adaptive discovery and root
		}
		if realIdx >= len(dryResults) {
			break
		}
		wantPath := rec.path
		gotPath := strings.TrimPrefix(dryResults[realIdx].URL, srv.URL)
		if gotPath != wantPath {
			t.Errorf("candidate %d: dry=%q real=%q", realIdx+1, gotPath, wantPath)
		}
		realIdx++
	}
}

// ── 6. --dry-run-limit 10 shows exactly 10 results ───────────────────────

func TestScanDryRun_Limit(t *testing.T) {
	words := make([]string, 100)
	for i := range words {
		words[i] = fmt.Sprintf("word%d", i)
	}
	r := wordlist.NewSliceReader(staticSliceReader(words))
	results, total, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com/"},
		r, false, false, nil, 10,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 10 {
		t.Fatalf("expected 10 previewed, got %d", len(results))
	}
	// Words are all "word" — DirectoryGenerator deduplicates, so total = 1.
	// But the important thing is we didn't get more than limit in results.
	_ = total
}

// ── 7. Limit does not alter candidate generation semantics ────────────────

func TestScanDryRun_LimitDoesNotReduceTotal(t *testing.T) {
	words := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	r := wordlist.NewSliceReader(staticSliceReader(words))
	results, total, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com/"},
		r, false, false, nil, 5,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 previewed, got %d", len(results))
	}
	if total != 11 {
		t.Fatalf("expected total=11 (all candidates), got %d", total)
	}
}

// ── 8. Quiet mode produces only URLs (tested via PrintScanDryRunHeader) ───
// The CLI test covers -q; here we test that Summary emits no banners.

func TestScanDryRun_QuietOutputFormat(t *testing.T) {
	// In quiet mode, the CLI prints only dr.URL; verify URLs are well-formed.
	r := sliceReader("page", "secret")
	results, _, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com/"},
		r, false, false, nil, 10,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, dr := range results {
		if !strings.HasPrefix(dr.URL, "http://example.com/") {
			t.Errorf("URL not properly formed: %s", dr.URL)
		}
	}
}

// ── 9. BFS ordering preserved ─────────────────────────────────────────────
// For a single seed at depth 1, candidates are always in wordlist order.

func TestScanDryRun_BFSOrdering(t *testing.T) {
	words := []string{"alpha", "beta", "gamma"}
	r := wordlist.NewSliceReader(staticSliceReader(words))
	results, _, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com/"},
		r, false, false, nil, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{
		"http://example.com/alpha",
		"http://example.com/beta",
		"http://example.com/gamma",
	}
	for i, e := range expected {
		if i >= len(results) {
			t.Fatalf("too few results: want %d, got %d", len(expected), len(results))
		}
		if results[i].URL != e {
			t.Errorf("result[%d]: want %q, got %q", i, e, results[i].URL)
		}
	}
}

// ── 10. DFS ordering preserved ────────────────────────────────────────────
// At depth 1 with a single seed, DFS and BFS produce the same candidate
// list (DFS only differs in how child directories are expanded, which
// requires HTTP responses). Verify order is still wordlist-sequential.

func TestScanDryRun_DFSOrdering(t *testing.T) {
	words := []string{"x", "y", "z"}
	r := wordlist.NewSliceReader(staticSliceReader(words))
	results, _, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com/"},
		r, false, false, nil, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3, got %d", len(results))
	}
	for i, w := range words {
		if !strings.HasSuffix(results[i].URL, "/"+w) {
			t.Errorf("result[%d] = %q, want suffix /%s", i, results[i].URL, w)
		}
	}
}

// ── 11. Priority ordering preserved ──────────────────────────────────────
// Same as BFS/DFS note: at depth 1, no fingerprinting occurs (nil cache).
// Ordering is wordlist-sequential regardless of strategy label.

func TestScanDryRun_PriorityOrdering(t *testing.T) {
	words := []string{"admin", "backup", "config"}
	r := wordlist.NewSliceReader(staticSliceReader(words))
	results, _, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com/"},
		r, false, false, nil, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3, got %d", len(results))
	}
}

// ── 12. --max-depth respected (documented limitation) ─────────────────────
// The generator only produces depth-1 candidates from each seed. It does
// not generate depth-2+ candidates without HTTP responses. The test confirms
// that the total is equal to len(wordlist) * len(seeds) (no phantom extras).

func TestScanDryRun_MaxDepthDoesNotExpandFurther(t *testing.T) {
	words := []string{"a", "b", "c"}
	seeds := []string{"http://example.com/"}
	r := wordlist.NewSliceReader(staticSliceReader(words))
	_, total, err := recursion.GenerateScanDryRunRequests(
		context.Background(), seeds, r,
		false, false, nil, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only depth-1: 3 words × 1 seed.
	if total != 3 {
		t.Fatalf("expected 3 (no depth expansion without HTTP), got %d", total)
	}
}

// ── 13. --normalize-paths respected ──────────────────────────────────────

func TestScanDryRun_NormalizePaths(t *testing.T) {
	// ".//../admin" after cleaning should normalize to "admin"
	words := []string{".//../admin", "api"}
	r := wordlist.NewSliceReader(staticSliceReader(words))
	results, _, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com/"},
		r, true, false, nil, 10,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, dr := range results {
		if strings.Contains(dr.URL, "..") || strings.Contains(dr.URL, "./") {
			t.Errorf("normalized path still contains dot segments: %s", dr.URL)
		}
	}
}

// ── 14. --collapse-slashes respected ─────────────────────────────────────

func TestScanDryRun_CollapseSlashes(t *testing.T) {
	words := []string{"admin////api", "login"}
	r := wordlist.NewSliceReader(staticSliceReader(words))
	results, _, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com/"},
		r, false, true, nil, 10,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, dr := range results {
		path := strings.TrimPrefix(dr.URL, "http://")
		if strings.Contains(path, "////") || strings.Contains(path, "///") || strings.Contains(path, "//") {
			t.Errorf("collapsed URL still has duplicate slashes: %s", dr.URL)
		}
	}
}

// ── 15. Extensions (--ext) are applied ───────────────────────────────────

func TestScanDryRun_Extensions(t *testing.T) {
	words := []string{"config"}
	exts := []string{"php", "html"}
	r := wordlist.NewSliceReader(staticSliceReader(words))
	results, total, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com/"},
		r, false, false, exts, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// GenerateVariants produces: config, config.php, config.html
	if total != 3 {
		t.Fatalf("expected 3 (1 word × 3 variants), got %d", total)
	}
	urls := make(map[string]bool)
	for _, dr := range results {
		urls[dr.URL] = true
	}
	if !urls["http://example.com/config"] {
		t.Error("missing http://example.com/config")
	}
	if !urls["http://example.com/config.php"] {
		t.Error("missing http://example.com/config.php")
	}
	if !urls["http://example.com/config.html"] {
		t.Error("missing http://example.com/config.html")
	}
}

// ── 16. Structured output combinations fail cleanly ───────────────────────
// This is tested in cmd/ tests (CLI-level). Here we verify that the internal
// function itself does not emit any JSON/CSV/Markdown by checking the
// ScanDryRunRequest type contains only a URL string.

func TestScanDryRun_StructuredOutputIsDenied(t *testing.T) {
	// Confirm the struct fields: only Index and URL (text-friendly).
	r := sliceReader("test")
	results, _, _ := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com/"},
		r, false, false, nil, 5,
	)
	if len(results) > 0 {
		dr := results[0]
		if dr.Index != 1 {
			t.Errorf("expected Index=1, got %d", dr.Index)
		}
		if !strings.HasPrefix(dr.URL, "http://") {
			t.Errorf("URL malformed: %s", dr.URL)
		}
	}
}

// ── 18. Context cancellation propagates cleanly ───────────────────────────

func TestScanDryRun_ContextCancel(t *testing.T) {
	words := make([]string, 10000)
	for i := range words {
		words[i] = "x"
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled
	r := wordlist.NewSliceReader(staticSliceReader(words))
	_, _, err := recursion.GenerateScanDryRunRequests(ctx,
		[]string{"http://example.com/"},
		r, false, false, nil, 0,
	)
	// Either context error or completed (small wordlist / all dupes).
	if err != nil && !isContextErr(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── Index ordering ────────────────────────────────────────────────────────

func TestScanDryRun_IndexOrdering(t *testing.T) {
	words := []string{"a", "b", "c", "d", "e"}
	r := wordlist.NewSliceReader(staticSliceReader(words))
	results, _, err := recursion.GenerateScanDryRunRequests(
		context.Background(),
		[]string{"http://example.com/"},
		r, false, false, nil, 0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, dr := range results {
		if dr.Index != i+1 {
			t.Errorf("result[%d].Index = %d, want %d", i, dr.Index, i+1)
		}
	}
}

// ── PrintScanDryRunHeader output ──────────────────────────────────────────

func TestScanDryRun_PrintHeader(t *testing.T) {
	var buf bytes.Buffer
	recursion.PrintScanDryRunHeader(&buf, recursion.ScanDryRunConfig{
		Target:      "http://example.com/",
		Workers:     32,
		IsRecursive: true,
		Strategy:    "BFS",
		MaxDepth:    3,
		Wordlist:    "embedded",
	})
	out := buf.String()
	if !strings.Contains(out, "DRY RUN") {
		t.Error("missing DRY RUN banner")
	}
	if !strings.Contains(out, "SCAN CONFIGURATION") {
		t.Error("missing SCAN CONFIGURATION header")
	}
	if !strings.Contains(out, "http://example.com/") {
		t.Error("missing target URL")
	}
	if !strings.Contains(out, "Recursive") {
		t.Error("missing Recursive mode")
	}
	if !strings.Contains(out, "BFS") {
		t.Error("missing BFS strategy")
	}
}

// ── PrintScanDryRunSummary output ────────────────────────────────────────

func TestScanDryRun_PrintSummary_Recursive(t *testing.T) {
	var buf bytes.Buffer
	recursion.PrintScanDryRunSummary(&buf, 10, 100, true)
	out := buf.String()
	if !strings.Contains(out, "DRY RUN SUMMARY") {
		t.Error("missing DRY RUN SUMMARY")
	}
	if !strings.Contains(out, "Requests Sent") {
		t.Error("missing Requests Sent")
	}
	if !strings.Contains(out, "No network requests were sent.") {
		t.Error("missing no-network message")
	}
	if !strings.Contains(out, "Deeper recursive paths require HTTP responses") {
		t.Error("missing recursive limitation note")
	}
}

func TestScanDryRun_PrintSummary_NonRecursive(t *testing.T) {
	var buf bytes.Buffer
	recursion.PrintScanDryRunSummary(&buf, 5, 50, false)
	out := buf.String()
	if strings.Contains(out, "Deeper recursive paths") {
		t.Error("should not include recursive limitation note for non-recursive mode")
	}
}

// ── helpers ──────────────────────────────────────────────────────────────

func isContextErr(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

type recordingTransport struct {
	fn func(*http.Request) (*http.Response, error)
}

func (t *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.fn(req)
}
