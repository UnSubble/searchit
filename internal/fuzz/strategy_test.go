package fuzz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/stats"
)

func TestStrategies(t *testing.T) {
	// Setup a mock HTTP server
	var requestedMutex sync.Mutex
	var requestedPaths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedMutex.Lock()
		requestedPaths = append(requestedPaths, r.URL.Path)
		requestedMutex.Unlock()

		// Success conditions
		path := r.URL.Path
		if path == "/admin" || path == "/api" || path == "/admin/users" || path == "/admin/users/profile" {
			if path == "/api" {
				w.Header().Set("Content-Type", "application/json")
			} else {
				w.Header().Set("Content-Type", "text/html")
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("found"))
			return
		}

		if path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /admin\n"))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	fooWords := []string{"admin", "api", "login"}
	barWords := []string{"users", "login", "uploads"}
	buzzWords := []string{"profile", "settings"}

	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	t.Run("bfs strategy", func(t *testing.T) {
		requestedMutex.Lock()
		requestedPaths = nil
		requestedMutex.Unlock()

		r := &fuzz.Runner{
			TargetURL: srv.URL + "/FOO/BAR/BUZZ",
			Method:    "GET",
			FooWords:  fooWords,
			BarWords:  barWords,
			BuzzWords: buzzWords,
			Client:    srv.Client(),
			FS:        fs,
			Threads:   4,
			Collector: stats.NewCollector(),
		}

		var results []fuzz.Result
		err := r.Run(context.Background(), "bfs", nil, func(res fuzz.Result) {
			results = append(results, res)
		})
		if err != nil {
			t.Fatalf("BFS Run failed: %v", err)
		}

		// Verify that fuzzed results are returned in exact wordlist order at each level
		if len(results) != 4 {
			t.Fatalf("expected 4 successful results in BFS, got %d: %v", len(results), results)
		}

		// BFS yields Level 1 first, then Level 2, then Level 3
		// Level 1 successes: admin, api
		// Level 2 successes: admin/users (from admin)
		// Level 3 successes: admin/users/profile (from admin/users)
		expectedPaths := []string{
			"/admin",
			"/api",
			"/admin/users",
			"/admin/users/profile",
		}
		for i, p := range expectedPaths {
			if !strings.HasSuffix(results[i].URL, p) {
				t.Errorf("expected result %d to end with %q, got %q", i, p, results[i].URL)
			}
		}

		// Verify request reduction: uploads was fuzzed under admin/api but not fuzzed deeper.
		// /api/uploads was requested but returned 404, so /api/uploads/profile was never fuzzed.
		requestedMutex.Lock()
		reqs := requestedPaths
		requestedMutex.Unlock()

		for _, req := range reqs {
			if strings.Contains(req, "uploads/profile") || strings.Contains(req, "login/profile") {
				t.Errorf("unwanted request made in BFS: %s", req)
			}
		}
	})

	t.Run("dfs strategy", func(t *testing.T) {
		requestedMutex.Lock()
		requestedPaths = nil
		requestedMutex.Unlock()

		r := &fuzz.Runner{
			TargetURL: srv.URL + "/FOO/BAR/BUZZ",
			Method:    "GET",
			FooWords:  fooWords,
			BarWords:  barWords,
			BuzzWords: buzzWords,
			Client:    srv.Client(),
			FS:        fs,
			Threads:   4,
			Collector: stats.NewCollector(),
		}

		var results []fuzz.Result
		err := r.Run(context.Background(), "dfs", nil, func(res fuzz.Result) {
			results = append(results, res)
		})
		if err != nil {
			t.Fatalf("DFS Run failed: %v", err)
		}

		if len(results) != 4 {
			t.Fatalf("expected 4 successful results in DFS, got %d: %v", len(results), results)
		}

		// DFS yields complete branches in order:
		// admin -> admin/users -> admin/users/profile -> api
		expectedPaths := []string{
			"/admin",
			"/admin/users",
			"/admin/users/profile",
			"/api",
		}
		for i, p := range expectedPaths {
			if !strings.HasSuffix(results[i].URL, p) {
				t.Errorf("expected result %d to end with %q, got %q", i, p, results[i].URL)
			}
		}
	})

	t.Run("smart strategy", func(t *testing.T) {
		requestedMutex.Lock()
		requestedPaths = nil
		requestedMutex.Unlock()

		cache := fingerprint.NewCache()

		r := &fuzz.Runner{
			TargetURL: srv.URL + "/FOO/BAR",
			Method:    "GET",
			FooWords:  fooWords,
			BarWords:  barWords,
			Client:    srv.Client(),
			FS:        fs,
			Threads:   4,
			Collector: stats.NewCollector(),
			Adaptive:  true,
			Cache:     cache,
		}

		var results []fuzz.Result
		err := r.Run(context.Background(), "smart", nil, func(res fuzz.Result) {
			results = append(results, res)
		})
		if err != nil {
			t.Fatalf("Smart Run failed: %v", err)
		}

		// Smart is BFS, so output order is: /admin, /api, /admin/users
		if len(results) != 3 {
			t.Fatalf("expected 3 results in Smart, got %d: %v", len(results), results)
		}
		expectedPaths := []string{
			"/admin",
			"/api",
			"/admin/users",
		}
		for i, p := range expectedPaths {
			if !strings.HasSuffix(results[i].URL, p) {
				t.Errorf("expected result %d to end with %q, got %q", i, p, results[i].URL)
			}
		}
	})

	t.Run("eager strategy", func(t *testing.T) {
		r := &fuzz.Runner{
			TargetURL: srv.URL + "/FOO/BAR",
			Method:    "GET",
			FooWords:  []string{"admin"},
			BarWords:  []string{"users"},
			Client:    srv.Client(),
			FS:        fs,
			Threads:   4,
			Collector: stats.NewCollector(),
		}

		var results []fuzz.Result
		err := r.Run(context.Background(), "eager", nil, func(res fuzz.Result) {
			results = append(results, res)
		})
		if err != nil {
			t.Fatalf("Eager Run failed: %v", err)
		}

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !strings.HasSuffix(results[0].URL, "/admin/users") {
			t.Errorf("expected URL ending in /admin/users, got %s", results[0].URL)
		}
	})

	t.Run("eager strategy with primaryChan", func(t *testing.T) {
		primary := make(chan string, 1)
		primary <- "myfuzz"
		close(primary)

		r := &fuzz.Runner{
			TargetURL: srv.URL + "/FUZZ/admin",
			Method:    "GET",
			Client:    srv.Client(),
			FS:        fs,
			Threads:   4,
			Collector: stats.NewCollector(),
		}

		var results []fuzz.Result
		err := r.Run(context.Background(), "eager", primary, func(res fuzz.Result) {
			results = append(results, res)
		})
		if err != nil {
			t.Fatalf("Eager primaryChan Run failed: %v", err)
		}

		// Since /myfuzz/admin returns 404, len(results) is 0 because of the filter, but the run completes successfully.
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("helper utilities", func(t *testing.T) {
		if d := fuzz.GetTargetDepth("http://localhost/FUZZ"); d != 0 {
			t.Errorf("expected depth 0, got %d", d)
		}
		if d := fuzz.GetTargetDepth("http://localhost/FOO"); d != 1 {
			t.Errorf("expected depth 1, got %d", d)
		}
		if d := fuzz.GetTargetDepth("http://localhost/FOO/BAR"); d != 2 {
			t.Errorf("expected depth 2, got %d", d)
		}
		if d := fuzz.GetTargetDepth("http://localhost/FOO/BAR/BUZZ"); d != 3 {
			t.Errorf("expected depth 3, got %d", d)
		}

		// TruncateTemplate edge cases
		if tr := fuzz.TruncateTemplate("http://localhost/FOO/BAR/BUZZ", 1); tr != "http://localhost/FOO" {
			t.Errorf("expected truncated depth 1, got %q", tr)
		}
		if tr := fuzz.TruncateTemplate("http://localhost/FOO/BAR/BUZZ", 2); tr != "http://localhost/FOO/BAR" {
			t.Errorf("expected truncated depth 2, got %q", tr)
		}
		if tr := fuzz.TruncateTemplate("http://localhost/FOO/BAR/BUZZ", 3); tr != "http://localhost/FOO/BAR/BUZZ" {
			t.Errorf("expected truncated depth 3, got %q", tr)
		}
	})
}
