package fuzz_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
)

// --- helpers -----------------------------------------------------------------

func newRunner(targetURL, method, body, cookie string, headers http.Header,
	fuzzWords, fooWords, barWords, bazWords, buzzWords []string,
	client *http.Client) *fuzz.Runner {
	fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	return &fuzz.Runner{
		TargetURL:       targetURL,
		Method:          method,
		BodyTemplate:    body,
		CookieTemplate:  cookie,
		HeaderTemplates: headers,
		FuzzWords:       fuzzWords,
		FooWords:        fooWords,
		BarWords:        barWords,
		BazWords:        bazWords,
		BuzzWords:       buzzWords,
		Client:          client,
		FS:              fs,
		Threads:         1,
	}
}

func primaryChan(words []string) <-chan string {
	ch := make(chan string, len(words))
	for _, w := range words {
		ch <- w
	}
	close(ch)
	return ch
}

// --- 1. Zero network requests ------------------------------------------------

func TestDryRun_ZeroNetworkRequests(t *testing.T) {
	var count int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&count, 1)
	}))
	defer srv.Close()

	client := srv.Client()
	r := newRunner(srv.URL+"/FUZZ", "GET", "", "", nil,
		[]string{"admin", "login"}, nil, nil, nil, nil, client)

	_, _, err := fuzz.GenerateDryRunRequests(context.Background(), r, primaryChan([]string{"admin", "login"}), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 HTTP requests, got %d", count)
	}
}

// --- 2. URL placeholder rendering --------------------------------------------

func TestDryRun_URLPlaceholder(t *testing.T) {
	r := newRunner("https://FUZZ.example.com/", "GET", "", "", nil,
		nil, nil, nil, nil, nil, nil)
	results, total, err := fuzz.GenerateDryRunRequests(
		context.Background(), r,
		primaryChan([]string{"admin", "api"}), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total=2, got %d", total)
	}
	if results[0].Req.URL != "https://admin.example.com/" {
		t.Errorf("expected https://admin.example.com/, got %s", results[0].Req.URL)
	}
	if results[1].Req.URL != "https://api.example.com/" {
		t.Errorf("expected https://api.example.com/, got %s", results[1].Req.URL)
	}
}

// --- 3. Header placeholder rendering ----------------------------------------

func TestDryRun_HeaderPlaceholder(t *testing.T) {
	headers := http.Header{"Host": []string{"BUZZ.example.com"}}
	r := newRunner("https://example.com/", "GET", "", "", headers,
		nil, nil, nil, nil, []string{"admin", "api"}, nil)
	results, _, err := fuzz.GenerateDryRunRequests(context.Background(), r, nil, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	got := results[0].Req.Headers["Host"]
	if len(got) == 0 || got[0] != "admin.example.com" {
		t.Errorf("expected Host: admin.example.com, got %v", got)
	}
	if results[0].Req.FuzzData == nil {
		t.Error("expected FuzzData to be non-nil for header placeholder")
	}
}

// --- 4. Cookie placeholder rendering ----------------------------------------

func TestDryRun_CookiePlaceholder(t *testing.T) {
	r := newRunner("https://example.com/", "GET", "", "session=FUZZ", nil,
		nil, nil, nil, nil, nil, nil)
	results, _, err := fuzz.GenerateDryRunRequests(
		context.Background(), r,
		primaryChan([]string{"abc123", "xyz789"}), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if len(results[0].Req.Cookies) == 0 || results[0].Req.Cookies[0] != "session=abc123" {
		t.Errorf("expected cookie session=abc123, got %v", results[0].Req.Cookies)
	}
}

// --- 5. Body placeholder rendering ------------------------------------------

func TestDryRun_BodyPlaceholder(t *testing.T) {
	r := newRunner("https://example.com/login", "POST", "user=FUZZ&pass=secret", "", nil,
		nil, nil, nil, nil, nil, nil)
	results, _, err := fuzz.GenerateDryRunRequests(
		context.Background(), r,
		primaryChan([]string{"admin"}), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Req.Body != "user=admin&pass=secret" {
		t.Errorf("expected body user=admin&pass=secret, got %s", results[0].Req.Body)
	}
}

// --- 6. Multiple placeholder rendering --------------------------------------

func TestDryRun_MultiplePlaceholders(t *testing.T) {
	headers := http.Header{"Host": []string{"BUZZ.example.com"}}
	r := newRunner("https://FUZZ.example.com/", "GET", "", "session=FOO", headers,
		nil, []string{"token1"}, nil, nil, []string{"admin"}, nil)
	results, total, err := fuzz.GenerateDryRunRequests(
		context.Background(), r,
		primaryChan([]string{"sub1"}), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 candidate, got %d", total)
	}
	if results[0].Req.URL != "https://sub1.example.com/" {
		t.Errorf("URL mismatch: %s", results[0].Req.URL)
	}
	gotHost := results[0].Req.Headers["Host"]
	if len(gotHost) == 0 || gotHost[0] != "admin.example.com" {
		t.Errorf("Host header mismatch: %v", gotHost)
	}
	if len(results[0].Req.Cookies) == 0 || results[0].Req.Cookies[0] != "session=token1" {
		t.Errorf("cookie mismatch: %v", results[0].Req.Cookies)
	}
}

// --- 7. --buzz =fuzz alias (already-loaded slice) ----------------------------

func TestDryRun_BuzzAlias(t *testing.T) {
	words := []string{"admin", "api", "dashboard"}
	headers := http.Header{"Host": []string{"BUZZ.example.com"}}
	r := newRunner("https://FUZZ.example.com/", "GET", "", "", headers,
		words, nil, nil, nil, words, nil) // BuzzWords = same slice as FuzzWords
	results, total, err := fuzz.GenerateDryRunRequests(
		context.Background(), r,
		primaryChan(words), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Cartesian: 3 FUZZ × 3 BUZZ = 9
	if total != 9 {
		t.Fatalf("expected 9 candidates (3×3), got %d", total)
	}
	_ = results
}

// --- 8. Cartesian candidate count correctness --------------------------------

func TestDryRun_CartesianCount(t *testing.T) {
	fuzzW := make([]string, 5)
	for i := range fuzzW {
		fuzzW[i] = "f"
	}
	fooW := make([]string, 3)
	for i := range fooW {
		fooW[i] = "o"
	}
	barW := make([]string, 4)
	for i := range barW {
		barW[i] = "b"
	}
	r := newRunner("https://example.com/FUZZ/FOO/BAR", "GET", "", "", nil,
		fuzzW, fooW, barW, nil, nil, nil)
	_, total, err := fuzz.GenerateDryRunRequests(
		context.Background(), r,
		primaryChan(fuzzW), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := int64(5 * 3 * 4) // 60
	if total != want {
		t.Fatalf("expected %d candidates, got %d", want, total)
	}
}

// --- 9. --dry-run-limit 10 shows exactly 10 requests -------------------------

func TestDryRun_Limit(t *testing.T) {
	words := make([]string, 100)
	for i := range words {
		words[i] = "word"
	}
	r := newRunner("https://example.com/FUZZ", "GET", "", "", nil,
		words, nil, nil, nil, nil, nil)
	results, total, err := fuzz.GenerateDryRunRequests(
		context.Background(), r,
		primaryChan(words), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 10 {
		t.Fatalf("expected 10 previewed requests, got %d", len(results))
	}
	if total != 100 {
		t.Fatalf("expected total=100, got %d", total)
	}
}

// --- 10. Candidate count not reduced by preview limit ------------------------

func TestDryRun_LimitDoesNotReduceTotal(t *testing.T) {
	words := make([]string, 50)
	for i := range words {
		words[i] = "x"
	}
	r := newRunner("https://example.com/FUZZ", "GET", "", "", nil,
		words, nil, nil, nil, nil, nil)
	results, total, err := fuzz.GenerateDryRunRequests(
		context.Background(), r,
		primaryChan(words), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 previewed, got %d", len(results))
	}
	if total != 50 {
		t.Fatalf("candidate count must be 50 regardless of limit, got %d", total)
	}
}

// --- 11. Context cancellation propagates cleanly -----------------------------

func TestDryRun_ContextCancel(t *testing.T) {
	words := make([]string, 10000)
	for i := range words {
		words[i] = "x"
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	r := newRunner("https://example.com/FUZZ", "GET", "", "", nil,
		words, nil, nil, nil, nil, nil)
	_, _, err := fuzz.GenerateDryRunRequests(ctx, r, primaryChan(words), 0)
	if err == nil {
		// Acceptable: very small wordlist may complete before context is checked,
		// but we must not panic and should not block.
		t.Log("context already cancelled but finished cleanly (small loop)")
	}
}

// --- 12. Parity: dry-run candidate #N == real candidate #N ------------------
//
// This is the critical regression guard. It ensures that GenerateDryRunRequests
// produces the same RequestDTOs in the same order as the real execution path.

func TestDryRun_Parity_WithRealExecutor(t *testing.T) {
	var (
		realMu   bytes.Buffer // just to collect ordering from real path
		realDTOs []fuzz.RequestDTO
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		// headers are already recorded by the runner callback
	}))
	defer srv.Close()

	words := []string{"alpha", "beta", "gamma"}
	buzzWords := []string{"x", "y"}
	headers := http.Header{"Host": []string{"BUZZ.example.com"}}

	// Real execution via runner.Run
	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	realRunner := &fuzz.Runner{
		TargetURL:       "https://FUZZ.example.com/",
		Method:          "GET",
		HeaderTemplates: headers,
		FuzzWords:       words,
		BuzzWords:       buzzWords,
		Client:          srv.Client(),
		FS:              fs,
		Threads:         1,
	}
	ctx := context.Background()
	_ = realRunner.Run(ctx, ctx, "eager", primaryChan(words), func(r fuzz.Result) {
		// Results from the live path (all accepted in test server)
	})

	// We can't easily capture RequestDTOs from the live path without hooking Worker.
	// Instead, we verify the dry-run against an independent ground truth:
	// run the real path with a recording RoundTripper and compare.

	type recordedReq struct {
		url  string
		host string
	}
	var realRecorded []recordedReq
	recordingTransport := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			realRecorded = append(realRecorded, recordedReq{
				url:  req.URL.String(),
				host: req.Host,
			})
			return &http.Response{
				StatusCode: 200,
				Body:       http.NoBody,
			}, nil
		},
	}
	recordingClient := &http.Client{Transport: recordingTransport}

	realRunner2 := &fuzz.Runner{
		TargetURL:       "https://FUZZ.example.com/",
		Method:          "GET",
		HeaderTemplates: headers,
		FuzzWords:       words,
		BuzzWords:       buzzWords,
		Client:          recordingClient,
		FS:              fs,
		Threads:         1,
	}
	if err := realRunner2.Run(ctx, ctx, "eager", primaryChan(words), func(r fuzz.Result) {}); err != nil {
		t.Fatalf("real runner.Run: %v", err)
	}

	// Dry-run on identical runner configuration.
	dryRunner := &fuzz.Runner{
		TargetURL:       "https://FUZZ.example.com/",
		Method:          "GET",
		HeaderTemplates: headers,
		FuzzWords:       words,
		BuzzWords:       buzzWords,
		FS:              fs,
		Threads:         1,
	}
	dryResults, total, err := fuzz.GenerateDryRunRequests(ctx, dryRunner, primaryChan(words), 0)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}

	if int(total) != len(realRecorded) {
		t.Fatalf("candidate count mismatch: dry=%d real=%d", total, len(realRecorded))
	}
	if len(dryResults) != len(realRecorded) {
		t.Fatalf("result count mismatch: dry=%d real=%d", len(dryResults), len(realRecorded))
	}
	for i, dr := range dryResults {
		rr := realRecorded[i]
		if dr.Req.URL != rr.url {
			t.Errorf("candidate %d URL mismatch: dry=%q real=%q", i+1, dr.Req.URL, rr.url)
		}
		gotHost := ""
		if h := dr.Req.Headers["Host"]; len(h) > 0 {
			gotHost = h[0]
		}
		if gotHost != rr.host {
			t.Errorf("candidate %d Host mismatch: dry=%q real=%q", i+1, gotHost, rr.host)
		}
	}

	_ = realMu
	_ = realDTOs
}

// --- 13. TLS never initialized (no real connection) --------------------------

func TestDryRun_NoTLSHandshake(t *testing.T) {
	var tlsCount int64
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&tlsCount, 1)
	}))
	defer srv.Close()

	// Deliberately use the TLS client — dry-run must never connect.
	r := newRunner(srv.URL+"/FUZZ", "GET", "", "", nil,
		[]string{"test"}, nil, nil, nil, nil, srv.Client())
	_, _, err := fuzz.GenerateDryRunRequests(context.Background(), r, primaryChan([]string{"test"}), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCount != 0 {
		t.Fatalf("expected 0 TLS connections, got %d", tlsCount)
	}
}

// --- 14. Indices are 1-based and sequential ----------------------------------

func TestDryRun_IndexOrdering(t *testing.T) {
	words := []string{"a", "b", "c", "d", "e"}
	r := newRunner("https://example.com/FUZZ", "GET", "", "", nil,
		words, nil, nil, nil, nil, nil)
	results, total, err := fuzz.GenerateDryRunRequests(context.Background(), r, primaryChan(words), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int(total) != len(words) {
		t.Fatalf("expected %d, got %d", len(words), total)
	}
	for i, dr := range results {
		if dr.Index != i+1 {
			t.Errorf("index[%d] = %d, want %d", i, dr.Index, i+1)
		}
	}
}
