package fuzz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/adaptive"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/stats"
)

func newTestFS(t *testing.T) *filter.FilterSuite {
	t.Helper()
	fs, err := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("filter.NewFilterSuite: %v", err)
	}
	return fs
}

// TestAdaptiveRunDrainsWordlistChannel verifies that when adaptive mode is enabled,
// all words from primaryChan are consumed and stored in FuzzWords before the
// traversal plan is built. Prior to the fix, FuzzWords was nil at plan-build
// time, causing words=[""] and Candidates=1.
func TestAdaptiveRunDrainsWordlistChannel(t *testing.T) {
	words := []string{"admin", "login", "api", "static", "uploads"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cache := fingerprint.NewCache()
	eng := adaptive.NewEngine(srv.URL+"/FUZZ", srv.Client(), cache, true /*quiet*/)

	primaryChan := make(chan string, len(words))
	for _, w := range words {
		primaryChan <- w
	}
	close(primaryChan)

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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {})
	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		t.Fatalf("Run() error: %v", err)
	}

	// All 5 words must have been processed as candidates.
	snap := collector.Snapshot()
	if snap.JobsProduced < int64(len(words)) {
		t.Errorf("expected at least %d jobs produced, got %d (adaptive wordlist drain bug: FuzzWords not populated from primaryChan)", len(words), snap.JobsProduced)
	}
}

// TestAdaptiveCandidateCountMatchesWordlist verifies that fuzz --adaptive produces
// Candidates == wordlist size, not 1 (the pre-fix behaviour).
func TestAdaptiveCandidateCountMatchesWordlist(t *testing.T) {
	const wordCount = 20

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cache := fingerprint.NewCache()
	eng := adaptive.NewEngine(srv.URL+"/FUZZ", srv.Client(), cache, true)

	primaryChan := make(chan string, wordCount)
	for i := 0; i < wordCount; i++ {
		primaryChan <- "word"
	}
	close(primaryChan)

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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_ = runner.Run(ctx, ctx, "eager", primaryChan, func(Result) {})

	snap := collector.Snapshot()
	if snap.JobsProduced == 1 {
		t.Errorf("got Candidates=1, want %d: adaptive mode is not draining primaryChan before buildTraversalPlan()", wordCount)
	}
	if snap.JobsProduced < int64(wordCount) {
		t.Errorf("got Candidates=%d, want %d", snap.JobsProduced, wordCount)
	}
}

// TestAdaptiveInfoHandlerCalledForDiscoveryMessages verifies that when InfoHandler
// is set on adaptive.Engine, it is called for [INFO] messages instead of writing
// directly to os.Stderr.
func TestAdaptiveInfoHandlerCalledForDiscoveryMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cache := fingerprint.NewCache()
	// quiet=false so [INFO] messages fire
	eng := adaptive.NewEngine(srv.URL, srv.Client(), cache, false)

	var callCount int64
	eng.InfoHandler = func(msg string) {
		atomic.AddInt64(&callCount, 1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = eng.Discover(ctx)

	// At minimum the two mandatory startup messages must have fired:
	// "[INFO] Adaptive mode enabled." and "[INFO] Discovering target..."
	if atomic.LoadInt64(&callCount) < 2 {
		t.Errorf("expected InfoHandler to be called at least 2 times, got %d; [INFO] messages may still be writing directly to os.Stderr", callCount)
	}
}

// TestRunnerInfoHandlerCalledForPriorityScores verifies that when InfoHandler is
// set on Runner, the priority scores block is emitted via InfoHandler rather than
// written directly to os.Stderr/os.Stdout.
func TestRunnerInfoHandlerCalledForPriorityScores(t *testing.T) {
	words := []string{"admin", "login"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cache := fingerprint.NewCache()
	// Engine is quiet; Runner is not — so priority score output fires from Runner.
	eng := adaptive.NewEngine(srv.URL+"/FUZZ", srv.Client(), cache, true)

	primaryChan := make(chan string, len(words))
	for _, w := range words {
		primaryChan <- w
	}
	close(primaryChan)

	var callCount int64
	runner := &Runner{
		TargetURL:      srv.URL + "/FUZZ",
		Client:         srv.Client(),
		FS:             newTestFS(t),
		Adaptive:       true,
		AdaptiveEngine: eng,
		Cache:          cache,
		Collector:      stats.NewCollector(),
		Threads:        2,
		Quiet:          false, // so priority scores are generated
		InfoHandler: func(msg string) {
			atomic.AddInt64(&callCount, 1)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = runner.Run(ctx, ctx, "eager", primaryChan, func(Result) {})

	// The priority scores block is emitted as a single InfoHandler call.
	if atomic.LoadInt64(&callCount) < 1 {
		t.Errorf("expected InfoHandler to be called for priority scores block, got %d calls; messages may still be going to stderr directly", callCount)
	}
}

// TestAdaptiveLiveFindingEmission verifies that accepted findings in adaptive mode
// are emitted live via yield callbacks during traversal execution, rather than being
// buffered and delayed until scan completion/draining.
func TestAdaptiveLiveFindingEmission(t *testing.T) {
	words := []string{"found1", "found2", "found3"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cache := fingerprint.NewCache()
	eng := adaptive.NewEngine(srv.URL+"/FUZZ", srv.Client(), cache, true)

	primaryChan := make(chan string, len(words))
	for _, w := range words {
		primaryChan <- w
	}
	close(primaryChan)

	var liveEmitted int64
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {
		if r.Accepted {
			atomic.AddInt64(&liveEmitted, 1)
		}
	})
	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		t.Fatalf("Run() error: %v", err)
	}

	if atomic.LoadInt64(&liveEmitted) != int64(len(words)) {
		t.Errorf("expected %d live emitted findings, got %d", len(words), liveEmitted)
	}
}
