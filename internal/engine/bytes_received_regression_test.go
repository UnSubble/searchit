package engine_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/stats"
)

func TestEngineWorker_BytesReceivedAccounting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/content-length":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", "20")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("12345678901234567890")) // 20 bytes

		case "/chunked":
			// Chunked transfer encoding (http.Response.ContentLength == -1)
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Transfer-Encoding", "chunked")
			w.WriteHeader(http.StatusOK)
			if f, ok := w.(http.Flusher); ok {
				_, _ = w.Write([]byte("chunk1-"))
				f.Flush()
				_, _ = w.Write([]byte("chunk2"))
				f.Flush()
			} // 13 bytes

		case "/404-no-length":
			// 404 with no explicit Content-Length (ContentLength == -1)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("NotFoundBody")) // 12 bytes

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	collector := stats.NewCollector()
	fs, err := filter.NewFilterSuite("200,404", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create filter suite: %v", err)
	}

	jobs := make(chan engine.Job, 10)
	results := make(chan engine.Result, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Launch worker with correct signature
	go engine.Worker(
		ctx, ctx, srv.Client(), fs,
		nil, nil, 0, nil,
		"GET", nil, nil, "",
		jobs, results, collector, nil,
	)

	jobs <- engine.Job{URL: srv.URL + "/content-length"}
	jobs <- engine.Job{URL: srv.URL + "/chunked"}
	jobs <- engine.Job{URL: srv.URL + "/404-no-length"}
	close(jobs)

	var resCount int
	for res := range results {
		resCount++
		if res.Err != nil {
			t.Errorf("unexpected job error for %s: %v", res.URL, res.Err)
		}
		if resCount == 3 {
			close(results)
		}
	}

	snap := collector.Snapshot()

	if snap.ResponsesReceived != 3 {
		t.Errorf("expected 3 responses received, got %d", snap.ResponsesReceived)
	}

	// BytesReceived must be >= 0 (MUST NOT BE NEGATIVE!)
	if snap.BytesReceived < 0 {
		t.Fatalf("CRITICAL BUG REPRODUCED: BytesReceived is negative: %d", snap.BytesReceived)
	}

	// Expected total bytes read = 20 + 13 + 12 = 45 bytes
	fmt.Printf("Total BytesReceived: %d\n", snap.BytesReceived)
	if snap.BytesReceived != 45 {
		t.Errorf("expected 45 bytes received, got %d", snap.BytesReceived)
	}
}
