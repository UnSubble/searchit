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

// TestStreamingMode_TotalAccumulatesFromZero verifies the accounting
// invariant for the explicit streaming path (StreamingMode=true):
//   - When called explicitly, totalWork accumulates per dispatched job
//   - completed <= totalWork throughout
//   - Progress percentage is always valid [0, 100]
//
// Note: since FileReader now implements Countable, normal file-wordlist scans
// use pre-counted mode (StreamingMode=false). This test exercises the streaming
// path directly (e.g. for future uncountable readers or adaptive discovery).
func TestStreamingMode_TotalAccumulatesFromZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // all filtered, no findings
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	words := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	wordCount := len(words)

	// Simulate streaming mode: no pre-count, totalWork starts at 0.
	collector := stats.NewCollector()
	collector.SetIsFinite(true)

	fs, err := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("filter suite: %v", err)
	}

	// Build primaryChan from a slice (simulating an uncountable stream).
	primaryChan := make(chan string, len(words))
	for _, w := range words {
		primaryChan <- w
	}
	close(primaryChan)

	runner := &Runner{
		TargetURL:     srv.URL + "/FUZZ",
		Method:        "GET",
		Client:        srv.Client(),
		FS:            fs,
		Threads:       2,
		Collector:     collector,
		StreamingMode: true, // explicit streaming: totalWork grows per job
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

	// Invariant 1: totalWork must equal jobsProduced (one per word, no extensions).
	if finalSnap.TotalWork != int64(wordCount) {
		t.Errorf("TotalWork=%d, want %d (one per word in streaming mode)", finalSnap.TotalWork, wordCount)
	}

	// Invariant 2: totalWork must never be 0 after scan with file wordlist.
	if finalSnap.TotalWork == 0 {
		t.Errorf("TotalWork is 0 after scan — streaming accumulation failed (was AddTotalCandidates a no-op?)")
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

	// Invariant 5: mid-run snapshot must also respect completed <= totalWork
	// (only meaningful if a mid-run snapshot was actually captured).
	if capturedMidRun {
		midCompleted := snapDuringRun.Tried + snapDuringRun.Skipped
		if snapDuringRun.TotalWork > 0 && midCompleted > snapDuringRun.TotalWork {
			t.Errorf("mid-run: completed (%d) > TotalWork (%d)", midCompleted, snapDuringRun.TotalWork)
		}
	}
}

// TestPreCountedMode_NoDoubleCount verifies that when StreamingMode is false
// (embedded/pre-counted wordlist), SetTotalCandidates at startup is the only
// writer and pushCandidate does NOT increment totalWork.
func TestPreCountedMode_StreamingModeFalse_NoDoubleCount(t *testing.T) {
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
	// Pre-counted: set total at startup.
	collector.SetTotalCandidates(int64(wordCount))

	fs, err := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("filter suite: %v", err)
	}

	// Load words into a channel.
	primaryChan := make(chan string, wordCount)
	for _, w := range words {
		primaryChan <- w
	}
	close(primaryChan)

	runner := &Runner{
		TargetURL:     srv.URL + "/FUZZ",
		Method:        "GET",
		Client:        srv.Client(),
		FS:            fs,
		Threads:       2,
		Collector:     collector,
		StreamingMode: false, // pre-counted: must NOT add to total in pushCandidate
	}

	err = runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {})
	if err != nil {
		t.Fatalf("Runner.Run failed: %v", err)
	}

	finalSnap := collector.Snapshot()

	// TotalWork must be exactly wordCount — no double-counting.
	if finalSnap.TotalWork != int64(wordCount) {
		t.Errorf("TotalWork=%d after pre-counted scan, want %d (must not double-count)", finalSnap.TotalWork, wordCount)
	}

	// completed <= totalWork.
	completed := finalSnap.Tried + finalSnap.Skipped
	if completed > finalSnap.TotalWork {
		t.Errorf("completed (%d) > TotalWork (%d)", completed, finalSnap.TotalWork)
	}
}
