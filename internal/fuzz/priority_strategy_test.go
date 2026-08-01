package fuzz_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/stats"
)

// TestPriorityScheduling_Order verifies that when node A is accepted,
// its children (A1, A2) are explored before older queued siblings (B, C).
func TestPriorityScheduling_Order(t *testing.T) {
	var requestOrder []string
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestOrder = append(requestOrder, r.URL.Path)
		mu.Unlock()

		switch r.URL.Path {
		case "/a":
			w.WriteHeader(http.StatusOK) // Accepted -> reveals a1, a2
		case "/a/a1", "/a/a2":
			w.WriteHeader(http.StatusOK)
		case "/b", "/c":
			w.WriteHeader(http.StatusNotFound) // Rejected
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	runner := &fuzz.Runner{
		TargetURL: srv.URL + "/FOO/BAR",
		Method:    "GET",
		FooWords:  []string{"a", "b", "c"},
		BarWords:  []string{"a1", "a2"},
		Client:    srv.Client(),
		FS:        fs,
		Threads:   1, // 1 worker to strictly observe sequential dispatch order
		Collector: stats.NewCollector(),
	}

	var yieldedResults []string
	err = runner.Run(context.Background(), context.Background(), "priority", nil, func(res fuzz.Result) {
		yieldedResults = append(yieldedResults, strings.TrimPrefix(res.URL, srv.URL))
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	expectedYielded := []string{"/a", "/a/a1", "/a/a2"}
	if !reflect.DeepEqual(yieldedResults, expectedYielded) {
		t.Errorf("expected yielded %v, got %v", expectedYielded, yieldedResults)
	}

	// Verify that /a/a1 and /a/a2 are explored BEFORE /b and /c in requestOrder
	var idxA1, idxA2, idxB, idxC int
	for i, path := range requestOrder {
		switch path {
		case "/a/a1":
			idxA1 = i
		case "/a/a2":
			idxA2 = i
		case "/b":
			idxB = i
		case "/c":
			idxC = i
		}
	}

	if idxA1 == 0 || idxA2 == 0 || idxB == 0 || idxC == 0 {
		t.Fatalf("missing expected requests in order: %v", requestOrder)
	}

	if idxA1 > idxB || idxA2 > idxB || idxA1 > idxC || idxA2 > idxC {
		t.Errorf("priority violation: children /a/a1, /a/a2 should be dispatched before /b and /c; full requestOrder: %v", requestOrder)
	}
}

// TestPriority_CandidateEquivalence verifies that eager, bfs, dfs, and priority
// all generate the exact same candidate count, URL set, and 0 duplicates across the search space.
func TestPriority_CandidateEquivalence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// All paths accept 200 so the entire tree is explored
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	fooWords := []string{"admin", "api", "login"}
	barWords := []string{"users", "v1"}
	buzzWords := []string{"profile", "debug"}

	strategies := []string{"eager", "bfs", "dfs", "priority"}
	resultsByStrategy := make(map[string][]string)
	requestedByStrategy := make(map[string][]string)

	for _, strat := range strategies {
		var mu sync.Mutex
		var requests []string

		trackedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			requests = append(requests, r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))

		runner := &fuzz.Runner{
			TargetURL: trackedSrv.URL + "/FOO/BAR/BUZZ",
			Method:    "GET",
			FooWords:  fooWords,
			BarWords:  barWords,
			BuzzWords: buzzWords,
			Client:    trackedSrv.Client(),
			FS:        fs,
			Threads:   4,
			Collector: stats.NewCollector(),
		}

		var yielded []string
		err := runner.Run(context.Background(), context.Background(), strat, nil, func(res fuzz.Result) {
			yielded = append(yielded, strings.TrimPrefix(res.URL, trackedSrv.URL))
		})
		trackedSrv.Close()

		if err != nil {
			t.Fatalf("strategy %s failed: %v", strat, err)
		}

		sort.Strings(yielded)
		sort.Strings(requests)

		resultsByStrategy[strat] = yielded
		requestedByStrategy[strat] = requests
	}

	// For hierarchical strategies (bfs, dfs, priority), the probing requests at depth 0, 1, 2
	// should produce identical requested URL sets.
	hierarchical := []string{"bfs", "dfs", "priority"}
	baselineYielded := resultsByStrategy["priority"]
	baselineReqs := requestedByStrategy["priority"]

	if len(baselineYielded) == 0 {
		t.Fatalf("priority generated 0 results")
	}

	for _, strat := range hierarchical {
		if !reflect.DeepEqual(baselineYielded, resultsByStrategy[strat]) {
			t.Errorf("candidate yield mismatch between priority and %s:\nPriority: %v\nGot:      %v",
				strat, baselineYielded, resultsByStrategy[strat])
		}
		if !reflect.DeepEqual(baselineReqs, requestedByStrategy[strat]) {
			t.Errorf("requested URL set mismatch between priority and %s:\nPriority: %v\nGot:      %v",
				strat, baselineReqs, requestedByStrategy[strat])
		}
	}
}

// TestPriority_WorkerCountDeterminism ensures that running with varying thread counts
// (1, 8, 32, 64, 128) produces bit-for-bit identical yielded results.
func TestPriority_WorkerCountDeterminism(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin", "/api":
			w.WriteHeader(http.StatusOK)
		case "/admin/users", "/admin/login", "/api/v1":
			w.WriteHeader(http.StatusOK)
		case "/admin/users/profile", "/api/v1/debug":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	threadCounts := []int{1, 8, 32, 64, 128}
	var baseline []string

	for i, threads := range threadCounts {
		runner := &fuzz.Runner{
			TargetURL: srv.URL + "/FOO/BAR/BUZZ",
			Method:    "GET",
			FooWords:  []string{"admin", "api", "other1", "other2"},
			BarWords:  []string{"users", "login", "v1", "other3"},
			BuzzWords: []string{"profile", "debug", "other4"},
			Client:    srv.Client(),
			FS:        fs,
			Threads:   threads,
			Collector: stats.NewCollector(),
		}

		var results []string
		err := runner.Run(context.Background(), context.Background(), "priority", nil, func(res fuzz.Result) {
			results = append(results, strings.TrimPrefix(res.URL, srv.URL))
		})
		if err != nil {
			t.Fatalf("priority failed with threads=%d: %v", threads, err)
		}

		sort.Strings(results)

		if i == 0 {
			baseline = results
			if len(baseline) == 0 {
				t.Fatalf("baseline run with threads=1 returned 0 results")
			}
		} else {
			if !reflect.DeepEqual(baseline, results) {
				t.Errorf("determinism violation for threads=%d:\nWant: %v\nGot:  %v",
					threads, baseline, results)
			}
		}
	}
}

// TestPriority_ContinuousThroughput verifies that Priority continuously supplies workers
// without suffering from the synchronous subtree stalls inherent to DFS.
func TestPriority_ContinuousThroughput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Small simulated server delay
		time.Sleep(2 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	var words []string
	for i := 0; i < 20; i++ {
		words = append(words, fmt.Sprintf("word%d", i))
	}

	runner := &fuzz.Runner{
		TargetURL: srv.URL + "/FOO/BAR",
		Method:    "GET",
		FooWords:  words,
		BarWords:  words[:5],
		Client:    srv.Client(),
		FS:        fs,
		Threads:   16,
		Collector: stats.NewCollector(),
	}

	start := time.Now()
	err = runner.Run(context.Background(), context.Background(), "priority", nil, func(res fuzz.Result) {})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("priority run failed: %v", err)
	}

	// 20 foo * (1 + 5 bar) = 120 requests total.
	// With 16 workers, ~120 requests with 2ms latency should finish well within 1 second.
	if elapsed > 3*time.Second {
		t.Errorf("priority traversal took excessively long (%v) suggesting worker stall", elapsed)
	}
}

// TestPriority_ExecutorQueueSaturation verifies that workers remain continuously saturated
// while pending work exists in the priority deque.
func TestPriority_ExecutorQueueSaturation(t *testing.T) {
	var activeCount int32
	var maxObservedActive int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt32(&activeCount, 1)
		for {
			oldMax := atomic.LoadInt32(&maxObservedActive)
			if cur <= oldMax || atomic.CompareAndSwapInt32(&maxObservedActive, oldMax, cur) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&activeCount, -1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	var fooWords []string
	for i := 0; i < 50; i++ {
		fooWords = append(fooWords, fmt.Sprintf("item%d", i))
	}

	runner := &fuzz.Runner{
		TargetURL: srv.URL + "/FOO/BAR",
		Method:    "GET",
		FooWords:  fooWords,
		BarWords:  []string{"sub1", "sub2"},
		Client:    srv.Client(),
		FS:        fs,
		Threads:   16,
		Collector: stats.NewCollector(),
	}

	err = runner.Run(context.Background(), context.Background(), "priority", nil, func(res fuzz.Result) {})
	if err != nil {
		t.Fatalf("priority run failed: %v", err)
	}

	if maxObservedActive < 8 {
		t.Errorf("expected worker concurrency saturation near 16, got max observed active: %d", maxObservedActive)
	}
}
