package engine_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/status"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func newApp(t *testing.T, excludeExpr string) *app.App {
	t.Helper()
	cfg := config.Default()
	if excludeExpr != "" {
		f, err := status.Parse(excludeExpr)
		if err != nil {
			t.Fatalf("status.Parse(%q): %v", excludeExpr, err)
		}
		cfg.Status.Exclude = f
	}
	return app.New(context.Background(), cfg)
}

func okServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ─────────────────────────────────────────────────────────────────────────────
// SliceProducer
// ─────────────────────────────────────────────────────────────────────────────

func TestSliceProducer_EmitsAllJobs(t *testing.T) {
	urls := []string{"http://a", "http://b", "http://c"}
	jobs := make(chan engine.Job, len(urls))

	err := engine.SliceProducer{URLs: urls}.Produce(context.Background(), jobs)
	if err != nil {
		t.Fatalf("Produce returned error: %v", err)
	}

	// Produce closes the channel; drain it.
	var got []string
	for j := range jobs {
		got = append(got, j.URL)
	}
	if len(got) != len(urls) {
		t.Fatalf("got %d jobs, want %d", len(got), len(urls))
	}
}

func TestSliceProducer_ClosesChannelWhenDone(t *testing.T) {
	jobs := make(chan engine.Job, 1)
	p := engine.SliceProducer{URLs: []string{"http://x"}}
	if err := p.Produce(context.Background(), jobs); err != nil {
		t.Fatalf("Produce returned error: %v", err)
	}

	// Drain all items; the range must terminate because Produce closed the channel.
	var count int
	for range jobs {
		count++
	}
	if count != 1 {
		t.Errorf("got %d jobs, want 1", count)
	}
}

func TestSliceProducer_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	// Large slice so Produce would block if cancellation is ignored.
	urls := make([]string, 1000)
	for i := range urls {
		urls[i] = fmt.Sprintf("http://host/%d", i)
	}

	jobs := make(chan engine.Job) // unbuffered — blocks if select misses ctx
	errCh := make(chan error, 1)
	go func() {
		errCh <- engine.SliceProducer{URLs: urls}.Produce(ctx, jobs)
	}()

	err := <-errCh
	if err == nil {
		t.Error("Produce returned nil, want ctx.Err()")
	}
}

func TestSliceProducer_EmptySlice(t *testing.T) {
	jobs := make(chan engine.Job, 1)
	err := engine.SliceProducer{}.Produce(context.Background(), jobs)
	if err != nil {
		t.Fatalf("empty Produce returned error: %v", err)
	}
	if _, ok := <-jobs; ok {
		t.Error("expected closed channel, got a value")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Worker
// ─────────────────────────────────────────────────────────────────────────────

func TestWorker_EmitsResultForAcceptedStatus(t *testing.T) {
	srv := okServer(t)
	a := newApp(t, "404")
	jobs := make(chan engine.Job, 1)
	results := make(chan engine.Result, 1)

	jobs <- engine.Job{URL: srv.URL}
	close(jobs)

	engine.Worker(context.Background(), a, jobs, results)
	close(results)

	var got []engine.Result
	for r := range results {
		got = append(got, r)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	if got[0].StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", got[0].StatusCode)
	}
	if got[0].URL != srv.URL {
		t.Errorf("URL = %q, want %q", got[0].URL, srv.URL)
	}
	if !got[0].Accepted {
		t.Error("Accepted = false, want true")
	}
}

func TestWorker_ExcludesStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	a := newApp(t, "404")
	jobs := make(chan engine.Job, 1)
	results := make(chan engine.Result, 1)

	jobs <- engine.Job{URL: srv.URL}
	close(jobs)

	engine.Worker(context.Background(), a, jobs, results)
	close(results)

	var got []engine.Result
	for r := range results {
		got = append(got, r)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	if got[0].Accepted {
		t.Error("got accepted result, want non-accepted for excluded status")
	}
}

func TestWorker_SkipsBadURL(t *testing.T) {
	a := newApp(t, "404")
	jobs := make(chan engine.Job, 1)
	results := make(chan engine.Result, 1)

	jobs <- engine.Job{URL: "://bad-url"}
	close(jobs)

	engine.Worker(context.Background(), a, jobs, results)
	close(results)

	var got []engine.Result
	for r := range results {
		got = append(got, r)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	if got[0].Accepted {
		t.Error("got accepted result for bad URL")
	}
	if got[0].Err == nil {
		t.Error("got nil error for bad URL, want non-nil")
	}
}

func TestWorker_SkipsUnreachableHost(t *testing.T) {
	a := newApp(t, "404")
	jobs := make(chan engine.Job, 1)
	results := make(chan engine.Result, 1)

	jobs <- engine.Job{URL: "http://127.0.0.1:1"}
	close(jobs)

	engine.Worker(context.Background(), a, jobs, results)
	close(results)

	var got []engine.Result
	for r := range results {
		got = append(got, r)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	if got[0].Accepted {
		t.Error("got accepted result for unreachable host")
	}
	if got[0].Err == nil {
		t.Error("got nil error for unreachable host, want non-nil")
	}
}

func TestWorker_ContentLengthInResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := []byte("hello")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			t.Errorf("handler Write: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	a := newApp(t, "404")
	jobs := make(chan engine.Job, 1)
	results := make(chan engine.Result, 1)

	jobs <- engine.Job{URL: srv.URL}
	close(jobs)

	engine.Worker(context.Background(), a, jobs, results)
	close(results)

	var got []engine.Result
	for r := range results {
		got = append(got, r)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	if got[0].Length != 5 {
		t.Errorf("Length = %d, want 5", got[0].Length)
	}
}

func TestWorker_CancelledContextSkipsRequests(t *testing.T) {
	srv := okServer(t)
	a := newApp(t, "404")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before any work

	jobs := make(chan engine.Job, 2)
	results := make(chan engine.Result, 2)
	jobs <- engine.Job{URL: srv.URL}
	jobs <- engine.Job{URL: srv.URL}
	close(jobs)

	engine.Worker(ctx, a, jobs, results)
	close(results)

	var got []engine.Result
	for r := range results {
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	for _, r := range got {
		if r.Accepted {
			t.Error("got accepted result with cancelled context")
		}
		if r.Err == nil {
			t.Error("got nil error with cancelled context")
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AcceptHeaders / AcceptContentLength placeholders
// ─────────────────────────────────────────────────────────────────────────────

func TestAcceptHeaders_AlwaysTrue(t *testing.T) {
	if !engine.AcceptHeaders(nil) {
		t.Error("AcceptHeaders returned false, want true")
	}
}

func TestAcceptContentLength_AlwaysTrue(t *testing.T) {
	for _, l := range []int64{-1, 0, 1, 999999} {
		if !engine.AcceptContentLength(l) {
			t.Errorf("AcceptContentLength(%d) = false, want true", l)
		}
	}
}

// ── Pool ──────────────────────────────────────────────────────────────────────

func TestStart_CollectsAllResults(t *testing.T) {
	const n = 10
	srv := okServer(t)
	a := newApp(t, "404")

	jobs := make(chan engine.Job, n)
	for i := range n {
		jobs <- engine.Job{URL: fmt.Sprintf("%s/%d", srv.URL, i)}
	}
	close(jobs)

	var got []engine.Result
	for r := range engine.Start(context.Background(), a, 5, jobs) {
		got = append(got, r)
	}
	if len(got) != n {
		t.Errorf("got %d results, want %d", len(got), n)
	}
}

func TestStart_ResultsChannelClosedAfterDone(t *testing.T) {
	a := newApp(t, "404")
	jobs := make(chan engine.Job)
	close(jobs)

	for range engine.Start(context.Background(), a, 3, jobs) {
	}
	// If we reach here the channel was closed and the loop terminated.
}

func TestStart_ExcludeFilterAppliedAcrossWorkers(t *testing.T) {
	var (
		mu sync.Mutex
		i  int
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if i%2 == 0 {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}

		i++
	}))
	t.Cleanup(srv.Close)

	const n = 8

	a := newApp(t, "404")

	jobs := make(chan engine.Job, n)

	for j := 0; j < n; j++ {
		jobs <- engine.Job{
			URL: fmt.Sprintf("%s/%d", srv.URL, j),
		}
	}

	close(jobs)

	for r := range engine.Start(context.Background(), a, 4, jobs) {
		if r.Accepted && r.StatusCode == http.StatusNotFound {
			t.Errorf("accepted 404 result leaked: %+v", r)
		}
		if !r.Accepted && r.StatusCode == http.StatusOK {
			t.Errorf("rejected 200 result: %+v", r)
		}
	}
}

func TestStart_URLsPreservedInResults(t *testing.T) {
	srv := okServer(t)
	urls := []string{srv.URL + "/a", srv.URL + "/b", srv.URL + "/c"}

	a := newApp(t, "404")
	jobs := make(chan engine.Job, len(urls))
	for _, u := range urls {
		jobs <- engine.Job{URL: u}
	}
	close(jobs)

	var got []string
	for r := range engine.Start(context.Background(), a, 3, jobs) {
		got = append(got, r.URL)
	}
	sort.Strings(got)
	sort.Strings(urls)
	for i, u := range urls {
		if got[i] != u {
			t.Errorf("URL[%d] = %q, want %q", i, got[i], u)
		}
	}
}

func TestStart_CancelledContextDrainsCleanly(t *testing.T) {
	srv := okServer(t)
	a := newApp(t, "404")

	ctx, cancel := context.WithCancel(context.Background())

	const n = 50
	jobs := make(chan engine.Job, n)
	for i := range n {
		jobs <- engine.Job{URL: fmt.Sprintf("%s/%d", srv.URL, i)}
	}
	close(jobs)

	results := engine.Start(ctx, a, 8, jobs)

	// Cancel after receiving the first result (or immediately if none come).
	cancel()

	// Must drain to completion without deadlock regardless of how many results
	// were emitted before cancellation.
	for range results {
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Scanner
// ─────────────────────────────────────────────────────────────────────────────

func TestScanner_Scan_CollectsResults(t *testing.T) {
	srv := okServer(t)
	urls := []string{srv.URL + "/1", srv.URL + "/2", srv.URL + "/3"}

	a := newApp(t, "404")
	s := engine.NewScanner(a)
	producer := engine.SliceProducer{URLs: urls}

	var got []engine.Result
	for r := range s.Scan(context.Background(), producer, 3) {
		got = append(got, r)
	}
	if len(got) != len(urls) {
		t.Errorf("got %d results, want %d", len(got), len(urls))
	}
}

func TestScanner_Scan_EmptyProducer(t *testing.T) {
	a := newApp(t, "404")
	s := engine.NewScanner(a)

	for range s.Scan(context.Background(), engine.SliceProducer{}, 4) {
		t.Error("expected no results from empty producer")
	}
}

func TestScanner_Scan_CancelledContext(t *testing.T) {
	srv := okServer(t)
	a := newApp(t, "404")
	s := engine.NewScanner(a)

	urls := make([]string, 100)
	for i := range urls {
		urls[i] = fmt.Sprintf("%s/%d", srv.URL, i)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Must drain without deadlock even with a cancelled context.
	for range s.Scan(ctx, engine.SliceProducer{URLs: urls}, 8) {
	}
}

func TestScanner_Scan_ExcludesStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	a := newApp(t, "404")
	s := engine.NewScanner(a)
	producer := engine.SliceProducer{URLs: []string{srv.URL + "/x", srv.URL + "/y"}}

	for r := range s.Scan(context.Background(), producer, 2) {
		t.Errorf("excluded result leaked: %+v", r)
	}
}
