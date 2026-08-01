package fuzz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/filter"
)

func setupTestRunnerAndExecutor(ctx, drainCtx context.Context, ts *httptest.Server, threads int) (*Runner, *Executor) {
	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	client := ts.Client()
	if tr, ok := client.Transport.(*http.Transport); ok {
		tr.DisableKeepAlives = true
	}

	r := &Runner{
		TargetURL: ts.URL + "/FUZZ",
		Client:    client,
		FS:        fs,
		Threads:   threads,
		FooWords:  []string{"foo"},
		BarWords:  []string{"bar"},
		BazWords:  []string{"baz"},
		BuzzWords: []string{"buzz"},
	}
	r.compiledReq = r.compileRequest()

	e := NewExecutor(ctx, drainCtx, client, fs, threads, 0, nil, nil, nil)
	return r, e
}

func TestRunEager_NormalExecution(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r, e := setupTestRunnerAndExecutor(ctx, ctx, ts, 2)
	defer e.Close()

	primaryChan := make(chan string, 1)
	primaryChan <- "testword"
	close(primaryChan)

	var yielded int32
	err := r.runEager(ctx, e, primaryChan, func(res Result) {
		atomic.AddInt32(&yielded, 1)
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&yielded) != 1 {
		t.Errorf("expected 1 yield, got %d", yielded)
	}
}

func TestRunEager_CancellationBeforeWorkerProcessesItem(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Cancel context immediately so workers exit instantly
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r, e := setupTestRunnerAndExecutor(ctx, ctx, ts, 2)
	defer e.Close()

	primaryChan := make(chan string, 10)
	for i := 0; i < 10; i++ {
		primaryChan <- "word"
	}
	close(primaryChan)

	var yielded int32
	err := r.runEager(ctx, e, primaryChan, func(res Result) {
		atomic.AddInt32(&yielded, 1)
	})

	if err != nil && err != context.Canceled {
		t.Errorf("expected nil or context.Canceled, got %v", err)
	}
	if atomic.LoadInt32(&yielded) > 0 {
		t.Errorf("expected 0 yields, got %d", yielded)
	}
}

func TestRunEager_CancellationWithQueuedReplyChannels(t *testing.T) {
	var handlerBlocked sync.WaitGroup
	handlerBlocked.Add(1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerBlocked.Wait()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, e := setupTestRunnerAndExecutor(ctx, ctx, ts, 2)
	defer e.Close()

	primaryChan := make(chan string, 100)
	for i := 0; i < 50; i++ {
		primaryChan <- "word"
	}
	close(primaryChan)

	done := make(chan struct{})
	go func() {
		// This will enqueue many jobs. The workers will block on the first two.
		// The producer will queue up reply channels in jobChan.
		// We then cancel the context.
		// runEager should exit promptly without deadlocking on the queued reply channels.
		_ = r.runEager(ctx, e, primaryChan, func(res Result) {})
		close(done)
	}()

	// Wait for a moment to ensure producer enqueues items
	time.Sleep(100 * time.Millisecond)
	cancel()
	handlerBlocked.Done() // Release any in-flight requests

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("runEager deadlocked waiting on orphaned reply channels")
	}
}

func TestRunEager_LargeQueue(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, e := setupTestRunnerAndExecutor(ctx, ctx, ts, 10) // 10 workers
	defer e.Close()

	primaryChan := make(chan string, 5000)
	for i := 0; i < 5000; i++ {
		primaryChan <- "word"
	}
	close(primaryChan)

	done := make(chan struct{})
	go func() {
		_ = r.runEager(ctx, e, primaryChan, func(res Result) {})
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel() // Cancel while in progress

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runEager deadlocked with large queue")
	}
}

func TestRunEager_NoGoroutineLeaks(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	func() {
		ctx, cancel := context.WithCancel(context.Background())
		r, e := setupTestRunnerAndExecutor(ctx, ctx, ts, 2)

		primaryChan := make(chan string, 10)
		for i := 0; i < 10; i++ {
			primaryChan <- "leaktest"
		}
		close(primaryChan)

		done := make(chan struct{})
		go func() {
			_ = r.runEager(ctx, e, primaryChan, func(res Result) {})
			close(done)
		}()

		time.Sleep(50 * time.Millisecond)
		cancel()
		e.Close()
		<-done
	}()

	// Wait for cleanup and close idle connections
	time.Sleep(200 * time.Millisecond)
	http.DefaultTransport.(*http.Transport).CloseIdleConnections()
	time.Sleep(100 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()

	// A small tolerance because runtime background goroutines might fluctuate.
	// We mainly want to ensure we aren't leaking workers or producer goroutines.
	if finalGoroutines > initialGoroutines+5 {
		buf := make([]byte, 2*1024*1024)
		n := runtime.Stack(buf, true)
		t.Fatalf("goroutine leak detected: started with %d, ended with %d\n%s", initialGoroutines, finalGoroutines, buf[:n])
	}
}
