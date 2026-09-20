package fuzz

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/adaptive"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/stats"
)

func cleanupAndVerifyNoGoroutineLeak(t *testing.T, initial int, client *http.Client, srv *httptest.Server, label string) {
	t.Helper()
	if srv != nil {
		srv.Close()
	}
	if client != nil {
		if tr, ok := client.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}
	http.DefaultTransport.(*http.Transport).CloseIdleConnections()

	// Wait up to 500ms for goroutines to drain
	deadline := time.Now().Add(500 * time.Millisecond)
	var final int
	for time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(20 * time.Millisecond)
		final = runtime.NumGoroutine()
		if final <= initial+2 {
			return
		}
	}

	buf := make([]byte, 2*1024*1024)
	n := runtime.Stack(buf, true)
	t.Fatalf("[%s] goroutine leak detected: started with %d, ended with %d\n%s", label, initial, final, string(buf[:n]))
}

// 1. Normal completion with small, medium, and larger wordlists
func TestAdaptiveLifecycle_NormalCompletion(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	var reqCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&reqCount, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	words := []string{"admin", "login", "dashboard", "api", "v1", "v2", "users", "settings"}
	primaryChan := make(chan string, len(words))
	for _, w := range words {
		primaryChan <- w
	}
	close(primaryChan)

	cache := fingerprint.NewCache()
	eng := adaptive.NewEngine(srv.URL+"/FUZZ", srv.Client(), cache, true)

	collector := stats.NewCollector()
	runner := &Runner{
		TargetURL:      srv.URL + "/FUZZ",
		Client:         srv.Client(),
		FS:             newTestFS(t),
		Adaptive:       true,
		AdaptiveEngine: eng,
		Cache:          cache,
		Collector:      collector,
		Threads:        4,
		Quiet:          true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var results []Result
	var mu sync.Mutex
	err := runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if atomic.LoadInt64(&reqCount) < int64(len(words)) {
		t.Errorf("expected at least %d requests, got %d", len(words), reqCount)
	}

	mu.Lock()
	resCount := len(results)
	mu.Unlock()
	if resCount != len(words) {
		t.Errorf("expected %d results, got %d", len(words), resCount)
	}

	cleanupAndVerifyNoGoroutineLeak(t, initialGoroutines, srv.Client(), srv, "NormalCompletion")
}

// 2. Cancellation mid-run
func TestAdaptiveLifecycle_CancellationMidRun(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	// 100 words so it takes enough time to cancel mid-run
	wordCount := 100
	primaryChan := make(chan string, wordCount)
	for i := 0; i < wordCount; i++ {
		primaryChan <- "test"
	}
	close(primaryChan)

	cache := fingerprint.NewCache()
	eng := adaptive.NewEngine(srv.URL+"/FUZZ", srv.Client(), cache, true)

	collector := stats.NewCollector()
	runner := &Runner{
		TargetURL:      srv.URL + "/FUZZ",
		Client:         srv.Client(),
		FS:             newTestFS(t),
		Adaptive:       true,
		AdaptiveEngine: eng,
		Cache:          cache,
		Collector:      collector,
		Threads:        2,
		Quiet:          true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	var cancelled atomic.Bool
	errChan := make(chan error, 1)

	go func() {
		err := runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {
			if !cancelled.Swap(true) {
				cancel()
			}
		})
		errChan <- err
	}()

	select {
	case err := <-errChan:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("expected nil or context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run timed out after cancellation")
	}

	cleanupAndVerifyNoGoroutineLeak(t, initialGoroutines, srv.Client(), srv, "CancellationMidRun")
}

// 3. Pre-cancelled context
func TestAdaptiveLifecycle_PreCancelledContext(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	primaryChan := make(chan string, 5)
	for i := 0; i < 5; i++ {
		primaryChan <- "test"
	}
	close(primaryChan)

	cache := fingerprint.NewCache()
	eng := adaptive.NewEngine(srv.URL+"/FUZZ", srv.Client(), cache, true)

	runner := &Runner{
		TargetURL:      srv.URL + "/FUZZ",
		Client:         srv.Client(),
		FS:             newTestFS(t),
		Adaptive:       true,
		AdaptiveEngine: eng,
		Cache:          cache,
		Collector:      stats.NewCollector(),
		Threads:        2,
		Quiet:          true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {})
	if err == nil && ctx.Err() == nil {
		t.Fatal("expected error on pre-cancelled context")
	}

	cleanupAndVerifyNoGoroutineLeak(t, initialGoroutines, srv.Client(), srv, "PreCancelledContext")
}

// 4. Empty wordlist
func TestAdaptiveLifecycle_EmptyWordlist(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	primaryChan := make(chan string)
	close(primaryChan)

	cache := fingerprint.NewCache()
	eng := adaptive.NewEngine(srv.URL+"/FUZZ", srv.Client(), cache, true)

	runner := &Runner{
		TargetURL:      srv.URL + "/FUZZ",
		Client:         srv.Client(),
		FS:             newTestFS(t),
		Adaptive:       true,
		AdaptiveEngine: eng,
		Cache:          cache,
		Collector:      stats.NewCollector(),
		Threads:        2,
		Quiet:          true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cleanupAndVerifyNoGoroutineLeak(t, initialGoroutines, srv.Client(), srv, "EmptyWordlist")
}

// 5. Very small wordlist (1 word)
func TestAdaptiveLifecycle_SingleWord(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	primaryChan := make(chan string, 1)
	primaryChan <- "single"
	close(primaryChan)

	cache := fingerprint.NewCache()
	eng := adaptive.NewEngine(srv.URL+"/FUZZ", srv.Client(), cache, true)

	runner := &Runner{
		TargetURL:      srv.URL + "/FUZZ",
		Client:         srv.Client(),
		FS:             newTestFS(t),
		Adaptive:       true,
		AdaptiveEngine: eng,
		Cache:          cache,
		Collector:      stats.NewCollector(),
		Threads:        2,
		Quiet:          true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var results []Result
	err := runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {
		results = append(results, r)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	cleanupAndVerifyNoGoroutineLeak(t, initialGoroutines, srv.Client(), srv, "SingleWord")
}

// 6. Target returns HTTP errors (500 Internal Server Error)
func TestAdaptiveLifecycle_TargetErrors(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))

	primaryChan := make(chan string, 5)
	for i := 0; i < 5; i++ {
		primaryChan <- "errtest"
	}
	close(primaryChan)

	cache := fingerprint.NewCache()
	eng := adaptive.NewEngine(srv.URL+"/FUZZ", srv.Client(), cache, true)

	runner := &Runner{
		TargetURL:      srv.URL + "/FUZZ",
		Client:         srv.Client(),
		FS:             newTestFS(t),
		Adaptive:       true,
		AdaptiveEngine: eng,
		Cache:          cache,
		Collector:      stats.NewCollector(),
		Threads:        2,
		Quiet:          true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cleanupAndVerifyNoGoroutineLeak(t, initialGoroutines, srv.Client(), srv, "TargetErrors")
}

// 7. Transport failure (connection refused / closed)
func TestAdaptiveLifecycle_TransportFailure(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	// Create listener then immediately close it to get connection refused
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadAddr := "http://" + l.Addr().String() + "/FUZZ"
	_ = l.Close()

	primaryChan := make(chan string, 5)
	for i := 0; i < 5; i++ {
		primaryChan <- "failtest"
	}
	close(primaryChan)

	cache := fingerprint.NewCache()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	eng := adaptive.NewEngine(deadAddr, client, cache, true)

	runner := &Runner{
		TargetURL:      deadAddr,
		Client:         client,
		FS:             newTestFS(t),
		Adaptive:       true,
		AdaptiveEngine: eng,
		Cache:          cache,
		Collector:      stats.NewCollector(),
		Threads:        2,
		Quiet:          true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Should not hang or leak goroutines
	_ = runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {})

	cleanupAndVerifyNoGoroutineLeak(t, initialGoroutines, client, nil, "TransportFailure")
}

// 8. Multi-level adaptive traversal (/FUZZ/FOO)
func TestAdaptiveLifecycle_MultiLevel(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	primaryChan := make(chan string, 3)
	primaryChan <- "api"
	primaryChan <- "admin"
	primaryChan <- "test"
	close(primaryChan)

	cache := fingerprint.NewCache()
	eng := adaptive.NewEngine(srv.URL+"/FUZZ/FOO", srv.Client(), cache, true)

	runner := &Runner{
		TargetURL:      srv.URL + "/FUZZ/FOO",
		FooWords:       []string{"users", "v1"},
		Client:         srv.Client(),
		FS:             newTestFS(t),
		Adaptive:       true,
		AdaptiveEngine: eng,
		Cache:          cache,
		Collector:      stats.NewCollector(),
		Threads:        4,
		Quiet:          true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var results []Result
	var mu sync.Mutex
	err := runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	count := len(results)
	mu.Unlock()
	if count == 0 {
		t.Error("expected at least some results for multi-level adaptive")
	}

	cleanupAndVerifyNoGoroutineLeak(t, initialGoroutines, srv.Client(), srv, "MultiLevel")
}
