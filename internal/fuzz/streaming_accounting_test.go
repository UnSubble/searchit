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

// TestFrozenDenominator_FuzzScanNeverMutatesTotal verifies that TotalWork
// is initialized before execution starts and remains constant throughout
// the entire fuzz scan.
func TestFrozenDenominator_FuzzScanNeverMutatesTotal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	words := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	wordCount := len(words)

	collector := stats.NewCollector()
	collector.SetIsFinite(true)
	collector.SetTotalCandidates(int64(wordCount))

	fs, err := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("filter suite: %v", err)
	}

	primaryChan := make(chan string, len(words))
	for _, w := range words {
		primaryChan <- w
	}
	close(primaryChan)

	runner := &Runner{
		TargetURL: srv.URL + "/FUZZ",
		Method:    "GET",
		Client:    srv.Client(),
		FS:        fs,
		Threads:   2,
		Collector: collector,
	}

	var snapDuringRun stats.Snapshot
	capturedMidRun := false

	err = runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {
		if !capturedMidRun {
			capturedMidRun = true
			snapDuringRun = collector.Snapshot()
		}
	})
	if err != nil {
		t.Fatalf("Runner.Run failed: %v", err)
	}

	finalSnap := collector.Snapshot()

	// Invariant 1: totalWork must stay exactly wordCount.
	if finalSnap.TotalWork != int64(wordCount) {
		t.Errorf("TotalWork=%d, want %d (must remain constant)", finalSnap.TotalWork, wordCount)
	}

	// Invariant 2: mid-run TotalWork must also be equal to wordCount.
	if capturedMidRun && snapDuringRun.TotalWork != int64(wordCount) {
		t.Errorf("mid-run TotalWork=%d, want %d", snapDuringRun.TotalWork, wordCount)
	}

	// Invariant 3: completed <= totalWork.
	completed := finalSnap.Tried + finalSnap.Skipped
	if completed > finalSnap.TotalWork {
		t.Errorf("completed (%d) > TotalWork (%d) — invariant violation", completed, finalSnap.TotalWork)
	}

	// Invariant 4: progress percentage is in [0, 100].
	if finalSnap.Progress < 0 || finalSnap.Progress > 100 {
		t.Errorf("Progress %.2f%% out of [0,100]", finalSnap.Progress)
	}
}

// TestPreCountedMode_NoDoubleCount verifies that SetTotalCandidates at startup
// sets the search space once and running the fuzz scan does NOT mutate totalWork.
func TestPreCountedMode_NoDoubleCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	words := []string{"a", "b", "c", "d", "e"}
	wordCount := len(words)

	collector := stats.NewCollector()
	collector.SetIsFinite(true)
	collector.SetTotalCandidates(int64(wordCount))

	fs, err := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("filter suite: %v", err)
	}

	primaryChan := make(chan string, wordCount)
	for _, w := range words {
		primaryChan <- w
	}
	close(primaryChan)

	runner := &Runner{
		TargetURL: srv.URL + "/FUZZ",
		Method:    "GET",
		Client:    srv.Client(),
		FS:        fs,
		Threads:   2,
		Collector: collector,
	}

	err = runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {})
	if err != nil {
		t.Fatalf("Runner.Run failed: %v", err)
	}

	finalSnap := collector.Snapshot()

	if finalSnap.TotalWork != int64(wordCount) {
		t.Errorf("TotalWork=%d after scan, want %d", finalSnap.TotalWork, wordCount)
	}

	completed := finalSnap.Tried + finalSnap.Skipped
	if completed > finalSnap.TotalWork {
		t.Errorf("completed (%d) > TotalWork (%d)", completed, finalSnap.TotalWork)
	}
}
