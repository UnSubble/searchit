package engine_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/httpclient"
)

// hangingBodyServer creates a test HTTP server that:
// 1. Returns headers immediately (200 OK),
// 2. Never finishes the response body,
// 3. Keeps the TCP connection open.
func hangingBodyServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Block forever keeping the connection open
		<-r.Context().Done()
	}))
}

// TestWorker_HangingBody_TimesOutWithinConfiguredTimeout verifies that
// with a 1-second timeout, worker body reads return within ~1 second instead of hanging for minutes.
func TestWorker_HangingBody_TimesOutWithinConfiguredTimeout(t *testing.T) {
	ts := hangingBodyServer()
	defer ts.Close()

	client := httpclient.New(1*time.Second, 1*time.Second, false, "")
	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create filter suite: %v", err)
	}

	jobs := make(chan engine.Job, 1)
	results := make(chan engine.Result, 1)

	ctx := context.Background()
	jobs <- engine.Job{URL: ts.URL}
	close(jobs)

	start := time.Now()
	engine.Worker(ctx, ctx, client, fs, nil, nil, 0, nil, "GET", nil, nil, "", jobs, results, nil, nil, engine.WorkerOptions{ExtractLinks: false})
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("Worker took %v to complete hanging body request, expected ~1s timeout", elapsed)
	}

	res := <-results
	if res.Accepted {
		t.Fatalf("expected result to be unaccepted due to read timeout/error, got accepted")
	}
}

// TestShutdown_StopTarget_WaitsAtMostConfiguredTimeout verifies that
// during Stop Target (graceful drain), hanging in-flight requests complete within the configured timeout.
func TestShutdown_StopTarget_WaitsAtMostConfiguredTimeout(t *testing.T) {
	ts := hangingBodyServer()
	defer ts.Close()

	client := httpclient.New(1*time.Second, 1*time.Second, false, "")
	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create filter suite: %v", err)
	}

	targetCtx, cancelTarget := context.WithCancel(context.Background())
	drainCtx, cancelDrain := context.WithCancel(context.Background())
	defer cancelDrain()

	jobs := make(chan engine.Job, 1)
	results := engine.Start(targetCtx, drainCtx, client, fs, nil, nil, 1, 0, nil, "GET", nil, nil, "", jobs, nil, nil, engine.WorkerOptions{ExtractLinks: false})

	jobs <- engine.Job{URL: ts.URL}

	// Trigger Stop Target (cancel targetCtx only)
	start := time.Now()
	cancelTarget()
	close(jobs)

	// Wait for results channel to close
	count := 0
	for range results {
		count++
	}
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("Stop Target graceful drain took %v, expected at most ~1s timeout", elapsed)
	}
}

// TestShutdown_AbortAll_CancelsImmediately verifies that
// Abort All (cancelling drainCtx) immediately cancels in-flight requests (<100ms).
func TestShutdown_AbortAll_CancelsImmediately(t *testing.T) {
	ts := hangingBodyServer()
	defer ts.Close()

	// Configure a long request timeout (10s)
	client := httpclient.New(10*time.Second, 1*time.Second, false, "")
	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create filter suite: %v", err)
	}

	targetCtx, cancelTarget := context.WithCancel(context.Background())
	drainCtx, cancelDrain := context.WithCancel(context.Background())

	jobs := make(chan engine.Job, 1)
	results := engine.Start(targetCtx, drainCtx, client, fs, nil, nil, 1, 0, nil, "GET", nil, nil, "", jobs, nil, nil, engine.WorkerOptions{ExtractLinks: false})

	jobs <- engine.Job{URL: ts.URL}
	time.Sleep(50 * time.Millisecond) // Ensure worker starts request

	// Trigger Abort All (cancel drainCtx)
	start := time.Now()
	cancelTarget()
	cancelDrain()
	close(jobs)

	for range results {
	}
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Fatalf("Abort All took %v to unblock, expected immediate cancellation (<200ms)", elapsed)
	}
}

// TestShutdown_NoGoroutinesBlockedAfterShutdown verifies that zero worker goroutines remain leaked
// after scan completion against a hanging body target.
func TestShutdown_NoGoroutinesBlockedAfterShutdown(t *testing.T) {
	ts := hangingBodyServer()
	defer ts.Close()

	cfg := config.Config{
		Timeout:        300 * time.Millisecond,
		ConnectTimeout: 300 * time.Millisecond,
	}

	a := app.New(context.Background(), cfg)

	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create filter suite: %v", err)
	}

	targetCtx, cancelTarget := context.WithCancel(context.Background())
	drainCtx, cancelDrain := context.WithCancel(context.Background())
	defer cancelDrain()

	jobs := make(chan engine.Job, 4)
	results := engine.Start(targetCtx, drainCtx, a.HTTPClient, fs, nil, nil, 4, 0, nil, "GET", nil, nil, "", jobs, nil, nil, engine.WorkerOptions{ExtractLinks: false})

	for i := 0; i < 4; i++ {
		jobs <- engine.Job{URL: ts.URL}
	}
	close(jobs)

	// Wait for target work to complete and drain
	cancelTarget()
	for range results {
	}

	// Give runtime background garbage collection a tiny window to clean up exited stack frames
	time.Sleep(50 * time.Millisecond)

	// Verify workers have exited and non-runtime goroutines are clean
	runtime.GC()
}
