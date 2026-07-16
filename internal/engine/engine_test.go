package engine_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
	"golang.org/x/time/rate"
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

func runWorker(ctx context.Context, a *app.App, jobs <-chan engine.Job, results chan<- engine.Result) {
	engine.Worker(ctx, a.HTTPClient, a.Config.Status.Exclude, nil, nil, nil, nil, 0, nil, "", nil, nil, nil, jobs, results, nil)
}

func startEngine(ctx context.Context, a *app.App, workers int, jobs <-chan engine.Job) <-chan engine.Result {
	return engine.Start(ctx, a.HTTPClient, a.Config.Status.Exclude, nil, nil, nil, nil, workers, 0, nil, "", nil, nil, nil, jobs, nil)
}

func newScanner(a *app.App) *engine.Scanner {
	return engine.NewScanner(a.HTTPClient, a.Config.Status.Exclude, nil, nil, nil, nil, 0, nil)
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

	runWorker(context.Background(), a, jobs, results)
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

	runWorker(context.Background(), a, jobs, results)
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

	runWorker(context.Background(), a, jobs, results)
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

	runWorker(context.Background(), a, jobs, results)
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

	runWorker(context.Background(), a, jobs, results)
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

	runWorker(ctx, a, jobs, results)
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
	for r := range startEngine(context.Background(), a, 5, jobs) {
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

	for range startEngine(context.Background(), a, 3, jobs) {
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

	for r := range startEngine(context.Background(), a, 4, jobs) {
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
	for r := range startEngine(context.Background(), a, 3, jobs) {
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

	results := startEngine(ctx, a, 8, jobs)

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
	s := newScanner(a)
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
	s := newScanner(a)

	for range s.Scan(context.Background(), engine.SliceProducer{}, 4) {
		t.Error("expected no results from empty producer")
	}
}

func TestScanner_Scan_CancelledContext(t *testing.T) {
	srv := okServer(t)
	a := newApp(t, "404")
	s := newScanner(a)

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
	s := newScanner(a)
	producer := engine.SliceProducer{URLs: []string{srv.URL + "/x", srv.URL + "/y"}}

	for r := range s.Scan(context.Background(), producer, 2) {
		t.Errorf("excluded result leaked: %+v", r)
	}
}

func TestWorker_Filters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/size100":
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(make([]byte, 100))
		case "/size200":
			w.Header().Set("Content-Length", "200")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(make([]byte, 200))
		case "/header-nginx":
			w.Header().Set("Server", "nginx")
			w.Header().Set("X-Powered-By", "PHP")
			w.WriteHeader(http.StatusOK)
		case "/header-apache":
			w.Header().Set("Server", "apache")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	a := newApp(t, "404")

	t.Run("include-size exact match", func(t *testing.T) {
		inc := size.MustParse("100")
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL + "/size100"}
		close(jobs)

		engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, inc, nil, nil, nil, 0, nil, "", nil, nil, nil, jobs, results, nil)
		r := <-results
		if !r.Accepted {
			t.Errorf("expected accepted=true for 100 byte response, got false")
		}
	})

	t.Run("include-size exact mismatch", func(t *testing.T) {
		inc := size.MustParse("200")
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL + "/size100"}
		close(jobs)

		engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, inc, nil, nil, nil, 0, nil, "", nil, nil, nil, jobs, results, nil)
		r := <-results
		if r.Accepted {
			t.Errorf("expected accepted=false for 100 byte response (included 200), got true")
		}
	})

	t.Run("include-size range match", func(t *testing.T) {
		inc := size.MustParse("50-150")
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL + "/size100"}
		close(jobs)

		engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, inc, nil, nil, nil, 0, nil, "", nil, nil, nil, jobs, results, nil)
		r := <-results
		if !r.Accepted {
			t.Errorf("expected accepted=true for size in range, got false")
		}
	})

	t.Run("exclude-size exact match", func(t *testing.T) {
		exc := size.MustParse("100")
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL + "/size100"}
		close(jobs)

		engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, nil, exc, nil, nil, 0, nil, "", nil, nil, nil, jobs, results, nil)
		r := <-results
		if r.Accepted {
			t.Errorf("expected accepted=false for excluded size, got true")
		}
	})

	t.Run("exclude-size range match", func(t *testing.T) {
		exc := size.MustParse("50-150")
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL + "/size100"}
		close(jobs)

		engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, nil, exc, nil, nil, 0, nil, "", nil, nil, nil, jobs, results, nil)
		r := <-results
		if r.Accepted {
			t.Errorf("expected accepted=false for size in excluded range, got true")
		}
	})

	t.Run("include-header matches", func(t *testing.T) {
		inc := []engine.HeaderFilter{{Name: "Server", Value: "nginx"}}
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL + "/header-nginx"}
		close(jobs)

		engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, nil, nil, inc, nil, 0, nil, "", nil, nil, nil, jobs, results, nil)
		r := <-results
		if !r.Accepted {
			t.Errorf("expected accepted=true for matching header, got false")
		}
	})

	t.Run("include-header mismatch", func(t *testing.T) {
		inc := []engine.HeaderFilter{{Name: "Server", Value: "nginx"}}
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL + "/header-apache"}
		close(jobs)

		engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, nil, nil, inc, nil, 0, nil, "", nil, nil, nil, jobs, results, nil)
		r := <-results
		if r.Accepted {
			t.Errorf("expected accepted=false for mismatching header, got true")
		}
	})

	t.Run("multiple include-header conditions (all match)", func(t *testing.T) {
		inc := []engine.HeaderFilter{
			{Name: "Server", Value: "nginx"},
			{Name: "X-Powered-By", Value: "PHP"},
		}
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL + "/header-nginx"}
		close(jobs)

		engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, nil, nil, inc, nil, 0, nil, "", nil, nil, nil, jobs, results, nil)
		r := <-results
		if !r.Accepted {
			t.Errorf("expected accepted=true when all headers match, got false")
		}
	})

	t.Run("multiple include-header conditions (one mismatch)", func(t *testing.T) {
		inc := []engine.HeaderFilter{
			{Name: "Server", Value: "nginx"},
			{Name: "X-Powered-By", Value: "Go"},
		}
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL + "/header-nginx"}
		close(jobs)

		engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, nil, nil, inc, nil, 0, nil, "", nil, nil, nil, jobs, results, nil)
		r := <-results
		if r.Accepted {
			t.Errorf("expected accepted=false when one of include headers mismatch, got true")
		}
	})

	t.Run("exclude-header match rejects", func(t *testing.T) {
		exc := []engine.HeaderFilter{{Name: "Server", Value: "nginx"}}
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL + "/header-nginx"}
		close(jobs)

		engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, nil, nil, nil, exc, 0, nil, "", nil, nil, nil, jobs, results, nil)
		r := <-results
		if r.Accepted {
			t.Errorf("expected accepted=false for excluded header match, got true")
		}
	})

	t.Run("case-insensitive header name matching", func(t *testing.T) {
		inc := []engine.HeaderFilter{{Name: "server", Value: "nginx"}}
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL + "/header-nginx"}
		close(jobs)

		engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, nil, nil, inc, nil, 0, nil, "", nil, nil, nil, jobs, results, nil)
		r := <-results
		if !r.Accepted {
			t.Errorf("expected accepted=true for case-insensitive header name match, got false")
		}
	})

	t.Run("case-insensitive header value matching", func(t *testing.T) {
		inc := []engine.HeaderFilter{{Name: "Server", Value: "NGINX"}}
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL + "/header-nginx"}
		close(jobs)

		engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, nil, nil, inc, nil, 0, nil, "", nil, nil, nil, jobs, results, nil)
		r := <-results
		if !r.Accepted {
			t.Errorf("expected accepted=true for case-insensitive header value match, got false")
		}
	})
}

func BenchmarkHeaderMatch(b *testing.B) {
	resp := &http.Response{
		Header: http.Header{
			"Server":       []string{"nginx"},
			"Content-Type": []string{"text/html"},
			"X-Powered-By": []string{"PHP"},
		},
	}
	inc := []engine.HeaderFilter{
		{Name: "Server", Value: "nginx"},
		{Name: "X-Powered-By", Value: "PHP"},
	}
	exc := []engine.HeaderFilter{
		{Name: "X-Header", Value: "val"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.AcceptHeaders(resp, inc, exc)
	}
}

func TestWorker_Delay(t *testing.T) {
	srv := okServer(t)
	a := newApp(t, "404")

	jobs := make(chan engine.Job, 2)
	results := make(chan engine.Result, 2)
	jobs <- engine.Job{URL: srv.URL + "/a"}
	jobs <- engine.Job{URL: srv.URL + "/b"}
	close(jobs)

	delay := 50 * time.Millisecond
	start := time.Now()
	engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, nil, nil, nil, nil, delay, nil, "", nil, nil, nil, jobs, results, nil)
	elapsed := time.Since(start)

	// Sleep 50ms after first, and 50ms after second. Total sleep >= 100ms.
	if elapsed < 100*time.Millisecond {
		t.Errorf("expected elapsed time to be at least 100ms with 50ms delay, got %v", elapsed)
	}
}

func TestWorker_RateLimit(t *testing.T) {
	srv := okServer(t)
	a := newApp(t, "404")

	jobs := make(chan engine.Job, 3)
	results := make(chan engine.Result, 3)
	jobs <- engine.Job{URL: srv.URL + "/a"}
	jobs <- engine.Job{URL: srv.URL + "/b"}
	jobs <- engine.Job{URL: srv.URL + "/c"}
	close(jobs)

	// Rate limit is 10 requests per second.
	// 3 requests should take around 200ms minimum because:
	// - req 1: Wait() takes 0 (burst = 1).
	// - req 2: Wait() takes 100ms.
	// - req 3: Wait() takes 100ms.
	// Total wait time = 200ms.
	limiter := rate.NewLimiter(rate.Limit(10), 1)

	start := time.Now()
	engine.Worker(context.Background(), a.HTTPClient, a.Config.Status.Exclude, nil, nil, nil, nil, 0, limiter, "", nil, nil, nil, jobs, results, nil)
	elapsed := time.Since(start)

	if elapsed < 200*time.Millisecond {
		t.Errorf("expected elapsed time to be at least 200ms with rate=10, got %v", elapsed)
	}

	// Test context cancellation in Rate Limit
	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		jobs2 := make(chan engine.Job, 1)
		results2 := make(chan engine.Result, 1)
		jobs2 <- engine.Job{URL: srv.URL + "/a"}
		close(jobs2)

		limiter2 := rate.NewLimiter(rate.Limit(1), 1)
		// Consumes the burst first
		_ = limiter2.Wait(context.Background())

		engine.Worker(ctx, a.HTTPClient, a.Config.Status.Exclude, nil, nil, nil, nil, 0, limiter2, "", nil, nil, nil, jobs2, results2, nil)
		// Should return immediately due to cancelled context without writing to results2
		select {
		case r := <-results2:
			t.Errorf("expected no result when context is cancelled, got %v", r)
		default:
		}
	})
}

func TestWorker_RequestManipulation(t *testing.T) {
	var recvMethod string
	var recvBody []byte
	var recvHeaders http.Header
	var recvCookies []*http.Cookie

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recvMethod = r.Method
		var err error
		recvBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		recvHeaders = r.Header
		recvCookies = r.Cookies()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := newApp(t, "404")

	jobs := make(chan engine.Job, 1)
	results := make(chan engine.Result, 1)
	jobs <- engine.Job{URL: srv.URL + "/test"}
	close(jobs)

	customHeaders := make(http.Header)
	customHeaders.Set("X-Custom-Req", "foobar")

	customCookies := []*http.Cookie{
		{Name: "session", Value: "secret123"},
	}

	engine.Worker(
		context.Background(),
		a.HTTPClient,
		a.Config.Status.Exclude,
		nil, nil, nil, nil,
		0,
		nil,
		"POST",
		[]byte("hello fuzz"),
		customHeaders,
		customCookies,
		jobs,
		results,
		nil,
	)

	<-results

	if recvMethod != "POST" {
		t.Errorf("expected Method POST, got %q", recvMethod)
	}
	if string(recvBody) != "hello fuzz" {
		t.Errorf("expected Body %q, got %q", "hello fuzz", string(recvBody))
	}
	if recvHeaders.Get("X-Custom-Req") != "foobar" {
		t.Errorf("expected Header X-Custom-Req=foobar, got %q", recvHeaders.Get("X-Custom-Req"))
	}
	if len(recvCookies) != 1 || recvCookies[0].Name != "session" || recvCookies[0].Value != "secret123" {
		t.Errorf("expected Cookie session=secret123, got %v", recvCookies)
	}
}
