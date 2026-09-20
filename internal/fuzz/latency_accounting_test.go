package fuzz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/stats"
)

func TestFuzzLatencySingleRecording(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	const N = 8
	words := []string{"w1", "w2", "w3", "w4", "w5", "w6", "w7", "w8"}

	collector := stats.NewCollector()
	collector.SetIsFinite(true)

	r := &fuzz.Runner{
		TargetURL: srv.URL + "/FUZZ",
		Method:    "GET",
		FuzzWords: words,
		Client:    srv.Client(),
		FS:        fs,
		Threads:   2,
		Collector: collector,
	}

	err = r.Run(context.Background(), context.Background(), "eager", nil, func(res fuzz.Result) {})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	snap := collector.Snapshot()

	if snap.RequestsSent != N {
		t.Errorf("RequestsSent: got %d, want %d", snap.RequestsSent, N)
	}
	if snap.ResponsesReceived != N {
		t.Errorf("ResponsesReceived: got %d, want %d", snap.ResponsesReceived, N)
	}

	// Invariant: latencyCount must equal number of completed requests N, NOT 2*N
	if snap.LatencyCount != N {
		t.Errorf("LatencyCount: got %d, want %d (must be N, not 2N)", snap.LatencyCount, N)
	}

	if snap.AverageLatency <= 0 {
		t.Errorf("AverageLatency: expected > 0, got %v", snap.AverageLatency)
	}
}

func TestFuzzLatencyWithRetries(t *testing.T) {
	var attempts int
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		current := attempts
		mu.Unlock()

		if current == 1 {
			// First attempt: close connection abruptly to trigger client.Do error & retry
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				_ = conn.Close()
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	collector := stats.NewCollector()
	collector.SetIsFinite(true)

	r := &fuzz.Runner{
		TargetURL: srv.URL + "/FUZZ",
		Method:    "GET",
		FuzzWords: []string{"retryme"},
		Client:    srv.Client(),
		FS:        fs,
		Threads:   1,
		Collector: collector,
	}

	err = r.Run(context.Background(), context.Background(), "eager", nil, func(res fuzz.Result) {})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	snap := collector.Snapshot()

	if snap.Retries < 1 {
		t.Logf("Note: retries = %d (server hijack may not have simulated network drop on this transport)", snap.Retries)
	}

	// Exactly 1 completed request -> exactly 1 latency measurement
	if snap.LatencyCount != 1 {
		t.Errorf("LatencyCount with retries: got %d, want 1", snap.LatencyCount)
	}
}
