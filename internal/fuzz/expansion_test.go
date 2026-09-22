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

func TestStrategyExtensionExpansion_Eager(t *testing.T) {
	var mu sync.Mutex
	var requestedPaths []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestedPaths = append(requestedPaths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	collector := stats.NewCollector()

	runner := &fuzz.Runner{
		TargetURL:  ts.URL + "/FUZZ",
		Method:     "GET",
		Extensions: []string{"", "php"},
		Client:     ts.Client(),
		FS:         fs,
		Threads:    1, // 1 thread for deterministic ordering
		Collector:  collector,
	}

	est := runner.EstimateCandidates(3)
	if est != 6 {
		t.Fatalf("expected estimated candidates 6, got %d", est)
	}

	primaryChan := buildChan([]string{"foo", "bar", "baz"})
	var results []fuzz.Result
	err := runner.Run(context.Background(), context.Background(), "eager", primaryChan, func(r fuzz.Result) {
		results = append(results, r)
	})
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}

	expected := []string{"/foo", "/foo.php", "/bar", "/bar.php", "/baz", "/baz.php"}
	mu.Lock()
	defer mu.Unlock()

	if len(requestedPaths) != len(expected) {
		t.Fatalf("expected %d requests, got %d: %v", len(expected), len(requestedPaths), requestedPaths)
	}
	for i, exp := range expected {
		if requestedPaths[i] != exp {
			t.Errorf("request[%d] = %q, want %q", i, requestedPaths[i], exp)
		}
	}
}

func TestStrategyExtensionExpansion_BFS(t *testing.T) {
	var mu sync.Mutex
	var requestedPaths []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestedPaths = append(requestedPaths, r.URL.Path)
		mu.Unlock()
		// Even if level 0 returns 404, level 1 must NOT be pruned
		if r.URL.Path == "/foo" || r.URL.Path == "/bar" || r.URL.Path == "/baz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	collector := stats.NewCollector()

	runner := &fuzz.Runner{
		TargetURL:  ts.URL + "/FUZZ",
		Method:     "GET",
		Extensions: []string{"", "php"},
		Client:     ts.Client(),
		FS:         fs,
		Threads:    1, // 1 worker for deterministic sequence
		Collector:  collector,
	}

	est := runner.EstimateCandidates(3)
	if est != 6 {
		t.Fatalf("expected estimated candidates 6, got %d", est)
	}

	primaryChan := buildChan([]string{"foo", "bar", "baz"})
	var results []fuzz.Result
	err := runner.Run(context.Background(), context.Background(), "bfs", primaryChan, func(r fuzz.Result) {
		results = append(results, r)
	})
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}

	// BFS Level 0: foo, bar, baz
	// BFS Level 1: foo.php, bar.php, baz.php
	expected := []string{"/foo", "/bar", "/baz", "/foo.php", "/bar.php", "/baz.php"}
	mu.Lock()
	defer mu.Unlock()

	if len(requestedPaths) != len(expected) {
		t.Fatalf("expected %d requests, got %d: %v", len(expected), len(requestedPaths), requestedPaths)
	}
	for i, exp := range expected {
		if requestedPaths[i] != exp {
			t.Errorf("request[%d] = %q, want %q", i, requestedPaths[i], exp)
		}
	}
}

func TestStrategyExtensionExpansion_DFS(t *testing.T) {
	var mu sync.Mutex
	var requestedPaths []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestedPaths = append(requestedPaths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)

	runner := &fuzz.Runner{
		TargetURL:  ts.URL + "/FUZZ",
		Method:     "GET",
		Extensions: []string{"", "php"},
		Client:     ts.Client(),
		FS:         fs,
		Threads:    1,
	}

	primaryChan := buildChan([]string{"foo", "bar", "baz"})
	err := runner.Run(context.Background(), context.Background(), "dfs", primaryChan, func(r fuzz.Result) {})
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}

	// DFS explores extension variants together with current candidate
	expected := []string{"/foo", "/foo.php", "/bar", "/bar.php", "/baz", "/baz.php"}
	mu.Lock()
	defer mu.Unlock()

	if len(requestedPaths) != len(expected) {
		t.Fatalf("expected %d requests, got %d: %v", len(expected), len(requestedPaths), requestedPaths)
	}
	for i, exp := range expected {
		if requestedPaths[i] != exp {
			t.Errorf("request[%d] = %q, want %q", i, requestedPaths[i], exp)
		}
	}
}

func TestStrategyExtensionExpansion_Priority(t *testing.T) {
	var mu sync.Mutex
	var requestedPaths []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestedPaths = append(requestedPaths, r.URL.Path)
		mu.Unlock()
		// Return 200 for foo, 404 for bar
		if r.URL.Path == "/foo" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)

	runner := &fuzz.Runner{
		TargetURL:  ts.URL + "/FUZZ",
		Method:     "GET",
		Extensions: []string{"", "php"},
		Client:     ts.Client(),
		FS:         fs,
		Threads:    1, // 1 worker ensures sequential deque processing
	}

	primaryChan := buildChan([]string{"foo", "bar"})
	err := runner.Run(context.Background(), context.Background(), "priority", primaryChan, func(r fuzz.Result) {})
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Initial deque: foo (ext=0), bar (ext=0), foo.php (ext=1), bar.php (ext=1)
	// When foo succeeds (200), foo.php is prioritized to the front.
	// So next executed is foo.php, then bar, then bar.php.
	expected := []string{"/foo", "/foo.php", "/bar", "/bar.php"}
	if len(requestedPaths) != len(expected) {
		t.Fatalf("expected %d requests, got %d: %v", len(expected), len(requestedPaths), requestedPaths)
	}
	for i, exp := range expected {
		if requestedPaths[i] != exp {
			t.Errorf("request[%d] = %q, want %q", i, requestedPaths[i], exp)
		}
	}
}

func TestStrategyExtensionExpansion_MultipleExtensions(t *testing.T) {
	var mu sync.Mutex
	var requestedPaths []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestedPaths = append(requestedPaths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)

	runner := &fuzz.Runner{
		TargetURL:  ts.URL + "/FUZZ",
		Method:     "GET",
		Extensions: []string{"", "php", "html", "txt"},
		Client:     ts.Client(),
		FS:         fs,
		Threads:    1,
	}

	primaryChan := buildChan([]string{"index"})
	err := runner.Run(context.Background(), context.Background(), "bfs", primaryChan, func(r fuzz.Result) {})
	if err != nil {
		t.Fatalf("runner.Run failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	expected := []string{"/index", "/index.php", "/index.html", "/index.txt"}
	if len(requestedPaths) != len(expected) {
		t.Fatalf("expected %d requests, got %d: %v", len(expected), len(requestedPaths), requestedPaths)
	}
	for i, exp := range expected {
		if requestedPaths[i] != exp {
			t.Errorf("request[%d] = %q, want %q", i, requestedPaths[i], exp)
		}
	}
}

func TestStrategyExtensionExpansion_Cancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)

	for _, strat := range []string{"eager", "bfs", "dfs", "priority"} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		runner := &fuzz.Runner{
			TargetURL:  ts.URL + "/FUZZ",
			Method:     "GET",
			Extensions: []string{"", "php"},
			Client:     ts.Client(),
			FS:         fs,
			Threads:    2,
		}

		words := make([]string, 50)
		for i := range words {
			words[i] = "item"
		}
		primaryChan := buildChan(words)

		_ = runner.Run(ctx, context.Background(), strat, primaryChan, func(r fuzz.Result) {})
	}
}

func TestStrategyExtensionExpansion_Adaptive(t *testing.T) {
	var mu sync.Mutex
	var requestedPaths []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestedPaths = append(requestedPaths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("X-Powered-By", "PHP/8.1")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	collector := stats.NewCollector()

	runner := &fuzz.Runner{
		TargetURL:  ts.URL + "/FUZZ",
		Method:     "GET",
		Extensions: []string{"", "php"},
		Client:     ts.Client(),
		FS:         fs,
		Threads:    1,
		Adaptive:   true,
		Quiet:      true,
		Collector:  collector,
	}

	primaryChan := buildChan([]string{"login", "admin"})
	err := runner.Run(context.Background(), context.Background(), "adaptive", primaryChan, func(r fuzz.Result) {})
	if err != nil {
		t.Fatalf("runner.Run adaptive failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	var candidatePaths []string
	for _, p := range requestedPaths {
		if p != "/robots.txt" && p != "/sitemap.xml" {
			candidatePaths = append(candidatePaths, p)
		}
	}

	// Both base words and .php variants must be executed (4 total candidates)
	if len(candidatePaths) != 4 {
		t.Fatalf("expected 4 candidate requests in adaptive mode, got %d: %v", len(candidatePaths), candidatePaths)
	}
}

func TestStrategyExtensionExpansion_Accounting(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)

	for _, strat := range []string{"eager", "bfs", "dfs", "priority"} {
		collector := stats.NewCollector()
		runner := &fuzz.Runner{
			TargetURL:  ts.URL + "/FUZZ",
			Method:     "GET",
			Extensions: []string{"", "php", "txt"},
			Client:     ts.Client(),
			FS:         fs,
			Threads:    2,
			Collector:  collector,
		}

		est := runner.EstimateCandidates(5)
		if est != 15 {
			t.Fatalf("[%s] expected 15 estimated candidates, got %d", strat, est)
		}
		collector.SetTotalCandidates(est)

		primaryChan := buildChan([]string{"w1", "w2", "w3", "w4", "w5"})
		var count int
		var countMu sync.Mutex
		err := runner.Run(context.Background(), context.Background(), strat, primaryChan, func(r fuzz.Result) {
			countMu.Lock()
			count++
			countMu.Unlock()
		})
		if err != nil {
			t.Fatalf("[%s] runner.Run failed: %v", strat, err)
		}

		if count != 15 {
			t.Errorf("[%s] expected 15 results, got %d", strat, count)
		}
	}
}
