package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/stats"
	"golang.org/x/time/rate"
)

// TestWorker_ProgressAccounting_DecrementsQueuedJobs verifies that the worker
// exactly decrements the queued jobs counter for every job processed.
func TestWorker_ProgressAccounting_DecrementsQueuedJobs(t *testing.T) {
	// Setup mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector := stats.NewCollector()

	// We simulate 10 queued jobs
	totalJobs := int64(10)
	collector.SetQueuedJobs(totalJobs)

	jobs := make(chan Job, totalJobs)
	results := make(chan Result, totalJobs)

	client := ts.Client()
	fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	var incHeaders []HeaderFilter
	var excHeaders []HeaderFilter

	// Start worker
	go Worker(
		ctx,
		client,
		fs,
		incHeaders,
		excHeaders,
		0, // delay
		(*rate.Limiter)(nil),
		"GET",
		nil,
		nil,
		"",
		jobs,
		results,
		collector,
	)

	// Send jobs
	for i := int64(0); i < totalJobs; i++ {
		jobs <- Job{URL: ts.URL}
	}
	close(jobs)

	// Consume results
	received := int64(0)
	for i := int64(0); i < totalJobs; i++ {
		select {
		case <-results:
			received++
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for result %d", i)
		}
	}

	if received != totalJobs {
		t.Fatalf("expected %d results, got %d", totalJobs, received)
	}

	// Verify progress accounting: queued jobs must be exactly 0
	snap := collector.Snapshot()
	if snap.QueuedJobs != 0 {
		t.Errorf("expected QueuedJobs to be 0 after %d jobs processed, but got %d", totalJobs, snap.QueuedJobs)
	}
}
