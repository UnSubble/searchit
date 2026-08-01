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
	collector.SetTotalCandidates(totalJobs)

	jobs := make(chan Job, totalJobs)
	results := make(chan Result, totalJobs)

	client := ts.Client()
	fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	var incHeaders []HeaderFilter
	var excHeaders []HeaderFilter

	// Start worker
	go Worker(
		ctx,
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
		nil,
		WorkerOptions{ExtractLinks: false},
	)

	// Send jobs
	for i := int64(0); i < totalJobs; i++ {
		jobs <- Job{URL: ts.URL}
		collector.RecordJobProduced()
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

	snap := collector.Snapshot()
	if snap.TotalCandidates != totalJobs {
		t.Errorf("Expected %d total jobs, got %d", totalJobs, snap.TotalCandidates)
	}

	if snap.JobsProduced != totalJobs {
		t.Errorf("expected JobsProduced to be %d, but got %d", totalJobs, snap.JobsProduced)
	}
}
