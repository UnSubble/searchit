package fuzz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/stats"
)

func TestFuzzWorker_Accounting_Invariants(t *testing.T) {
	// Setup mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector := stats.NewCollector()
	totalJobs := int64(10)
	collector.SetTotalCandidates(totalJobs)

	jobs := make(chan WorkItem, totalJobs)
	results := make(chan Result, totalJobs)

	client := ts.Client()
	fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)

	// Start worker
	go Worker(
		ctx,
		client,
		fs,
		0,   // delay
		nil, // limiter
		jobs,
		results,
		collector,
		nil,
	)

	// Send jobs
	for i := int64(0); i < totalJobs; i++ {
		jobs <- WorkItem{Req: RequestDTO{URL: ts.URL}}
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

	if received != totalJobs {
		t.Fatalf("expected %d results, got %d", totalJobs, received)
	}

	snap := collector.Snapshot()

	if snap.JobsProduced != totalJobs {
		t.Errorf("Expected %d JobsProduced, got %d", totalJobs, snap.JobsProduced)
	}

	// Invariant 2: Discovered (Findings) must equal the number of accepted jobs
	if snap.Discovered != totalJobs {
		t.Errorf("expected Discovered = %d, got %d", totalJobs, snap.Discovered)
	}
}
