package recursion_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
)

// normalizedResult represents the key observable properties of a scan discovery.
// We assert that these exact fields must match regardless of worker count.
type normalizedResult struct {
	URL        string
	StatusCode int
	Depth      uint16
	Accepted   bool
}

// randomJitter returns a random duration between 0 and maxMs to introduce timing noise.
// Uses crypto/rand to avoid global math/rand seed races and ensure concurrency safety.
func randomJitter(maxMs int64) time.Duration {
	if maxMs <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(maxMs))
	if err != nil {
		return 0
	}
	return time.Duration(n.Int64()) * time.Millisecond
}

func TestConcurrencyCorrectness_WorkerCounts(t *testing.T) {
	// 1. Setup mock routes and their status codes.
	// We intentionally include multi-depth nested directories to stress recursion logic.
	mockRoutes := map[string]int{
		"/static":              200,
		"/static/js":           200,
		"/static/js/app.js":    200,
		"/admin":               200,
		"/admin/panel":         200,
		"/admin/panel/users":   200,
		"/health":              200,
		"/redirect":            302,
		"/redirect/dest":       200,
		"/invalid-path-404":    404,
		"/another-invalid-404": 404,
	}

	// 2. Setup mock HTTP server with scheduling noise and random jitters.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Introduce artificial scheduling delay to maximize worker thread interleaving
		baseDelay := 2 * time.Millisecond
		switch r.URL.Path {
		case "/admin":
			baseDelay = 8 * time.Millisecond
		case "/static/js/app.js":
			baseDelay = 15 * time.Millisecond
		case "/health":
			baseDelay = 5 * time.Millisecond
		}
		time.Sleep(baseDelay + randomJitter(10))

		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Root"))
			return
		}

		pathWithQuery := r.URL.Path
		if r.URL.RawQuery != "" {
			pathWithQuery += "?" + r.URL.RawQuery
		}

		if statusVal, ok := mockRoutes[pathWithQuery]; ok {
			if statusVal == 302 {
				w.Header().Set("Location", "/redirect/dest")
			}
			w.WriteHeader(statusVal)
			_, _ = w.Write([]byte("Found"))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	// 3. Define candidates wordlist.
	// Includes duplicates, spaces, segments, and invalid segments.
	reader := staticReader{
		words: []string{
			"static",
			"js",
			"app.js",
			"admin",
			"panel",
			"users",
			"health",
			"redirect",
			"dest",
			"admin", // Duplicate
			"js",    // Duplicate
			"invalid-path-404",
			"another-invalid-404",
			".",          // Dot segment
			"..",         // Double dot segment
			"  static  ", // Whitespace padding
		},
	}

	// 4. Define our Golden Standard of expected normalized results.
	// Note: /redirect returns status code 200 because http.DefaultClient follows redirects by default.
	expectedResults := []normalizedResult{
		{URL: srv.URL, StatusCode: 200, Depth: 0, Accepted: true},
		{URL: srv.URL + "/admin", StatusCode: 200, Depth: 1, Accepted: true},
		{URL: srv.URL + "/admin/panel", StatusCode: 200, Depth: 2, Accepted: true},
		{URL: srv.URL + "/admin/panel/users", StatusCode: 200, Depth: 3, Accepted: true},
		{URL: srv.URL + "/health", StatusCode: 200, Depth: 1, Accepted: true},
		{URL: srv.URL + "/redirect", StatusCode: 200, Depth: 1, Accepted: true},
		{URL: srv.URL + "/redirect/dest", StatusCode: 200, Depth: 2, Accepted: true},
		{URL: srv.URL + "/static", StatusCode: 200, Depth: 1, Accepted: true},
		{URL: srv.URL + "/static/js", StatusCode: 200, Depth: 2, Accepted: true},
		{URL: srv.URL + "/static/js/app.js", StatusCode: 200, Depth: 3, Accepted: true},
	}
	sortNormalizedResults(expectedResults)

	workerCounts := []int{1, 2, 4, 8, 16, 32, 64, 128}

	for _, wc := range workerCounts {
		t.Run(fmt.Sprintf("Workers-%d", wc), func(t *testing.T) {
			stats.GlobalInstrumentation.Reset()
			atomic.StoreInt32(&stats.GlobalInstrumentation.Enabled, 1)

			excludeFilters, _ := status.Parse("404")
			recurseOnFilters, _ := status.Parse("200,302")

			manager := recursion.NewManager(
				http.DefaultClient,
				excludeFilters,
				reader,
				recursion.BFS,
				3, // maxDepth 3 to discover the third recursion layer (/admin/panel/users)
				recurseOnFilters,
				true, // normalizePaths
				true, // collapseSlashes
				nil, nil, nil, nil, 0, nil, nil,
			)

			ctx := context.Background()
			resultsChan := manager.Run(ctx, []string{srv.URL}, wc)

			var actual []normalizedResult
			for r := range resultsChan {
				actual = append(actual, normalizedResult{
					URL:        r.URL,
					StatusCode: r.StatusCode,
					Depth:      r.Depth,
					Accepted:   r.Accepted,
				})
			}
			sortNormalizedResults(actual)

			// Assert that actual discoveries match expected discoveries exactly.
			compareResultSets(t, expectedResults, actual)

			// Verify that the HTTP lifecycle invariant matches perfectly.
			verifyLifecycleInvariant(t)
		})
	}
}

func TestConcurrencyCorrectness_StressTest(t *testing.T) {
	// Setup mock server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Active delay with jitter to stress interleaving
		time.Sleep(1*time.Millisecond + randomJitter(5))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	reader := staticReader{
		words: []string{
			"a", "b", "c", "d", "e",
		},
	}

	expectedResults := []normalizedResult{
		{URL: srv.URL, StatusCode: 200, Depth: 0, Accepted: true},
		{URL: srv.URL + "/a", StatusCode: 200, Depth: 1, Accepted: true},
		{URL: srv.URL + "/b", StatusCode: 200, Depth: 1, Accepted: true},
		{URL: srv.URL + "/c", StatusCode: 200, Depth: 1, Accepted: true},
		{URL: srv.URL + "/d", StatusCode: 200, Depth: 1, Accepted: true},
		{URL: srv.URL + "/e", StatusCode: 200, Depth: 1, Accepted: true},
	}
	sortNormalizedResults(expectedResults)

	// Repeat the scan 50 times with 64 workers to verify lack of race conditions and determinism.
	const iterations = 50
	const workers = 64

	for i := 0; i < iterations; i++ {
		stats.GlobalInstrumentation.Reset()
		atomic.StoreInt32(&stats.GlobalInstrumentation.Enabled, 1)

		excludeFilters, _ := status.Parse("404")
		recurseOnFilters, _ := status.Parse("200")

		manager := recursion.NewManager(
			http.DefaultClient,
			excludeFilters,
			reader,
			recursion.BFS,
			1,
			recurseOnFilters,
			true,
			true,
			nil, nil, nil, nil, 0, nil, nil,
		)

		ctx := context.Background()
		resultsChan := manager.Run(ctx, []string{srv.URL}, workers)

		var actual []normalizedResult
		for r := range resultsChan {
			actual = append(actual, normalizedResult{
				URL:        r.URL,
				StatusCode: r.StatusCode,
				Depth:      r.Depth,
				Accepted:   r.Accepted,
			})
		}
		sortNormalizedResults(actual)

		compareResultSets(t, expectedResults, actual)
		verifyLifecycleInvariant(t)
	}
}

// Helpers

func sortNormalizedResults(results []normalizedResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].URL != results[j].URL {
			return results[i].URL < results[j].URL
		}
		return results[i].Depth < results[j].Depth
	})
}

func compareResultSets(t *testing.T, expected, actual []normalizedResult) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Errorf("Result set size mismatch: expected %d, got %d", len(expected), len(actual))
	}

	expectedMap := make(map[string]normalizedResult)
	for _, r := range expected {
		expectedMap[r.URL] = r
	}

	actualMap := make(map[string]normalizedResult)
	for _, r := range actual {
		actualMap[r.URL] = r
	}

	// 1. Check for missing
	for urlStr, exp := range expectedMap {
		act, found := actualMap[urlStr]
		if !found {
			t.Errorf("Missing expected discovery: %s", urlStr)
			continue
		}
		if exp.StatusCode != act.StatusCode {
			t.Errorf("Status code mismatch for %s: expected %d, got %d", urlStr, exp.StatusCode, act.StatusCode)
		}
		if exp.Depth != act.Depth {
			t.Errorf("Depth mismatch for %s: expected %d, got %d", urlStr, exp.Depth, act.Depth)
		}
		if exp.Accepted != act.Accepted {
			t.Errorf("Accepted status mismatch for %s: expected %t, got %t", urlStr, exp.Accepted, act.Accepted)
		}
	}

	// 2. Check for unexpected
	for urlStr := range actualMap {
		if _, found := expectedMap[urlStr]; !found {
			t.Errorf("Unexpected discovery: %s", urlStr)
		}
	}
}

func verifyLifecycleInvariant(t *testing.T) {
	t.Helper()
	jobsRecv := atomic.LoadInt64(&stats.GlobalInstrumentation.WorkerJobsRecv)
	reqsBuilt := atomic.LoadInt64(&stats.GlobalInstrumentation.RequestsBuilt)
	reqsSent := atomic.LoadInt64(&stats.GlobalInstrumentation.RequestsSent)
	respsRecv := atomic.LoadInt64(&stats.GlobalInstrumentation.ResponsesReceived)
	resultsProd := atomic.LoadInt64(&stats.GlobalInstrumentation.ResultsProduced)
	resultsCons := atomic.LoadInt64(&stats.GlobalInstrumentation.ResultsConsumed)

	if jobsRecv != reqsBuilt {
		t.Errorf("Lifecycle mismatch: Jobs Received (%d) != Requests Built (%d)", jobsRecv, reqsBuilt)
	}
	if reqsBuilt != reqsSent {
		t.Errorf("Lifecycle mismatch: Requests Built (%d) != Requests Sent (%d)", reqsBuilt, reqsSent)
	}
	if reqsSent != respsRecv {
		t.Errorf("Lifecycle mismatch: Requests Sent (%d) != Responses Received (%d)", reqsSent, respsRecv)
	}
	if resultsProd != respsRecv {
		t.Errorf("Lifecycle mismatch: Results Produced (%d) != Responses Received (%d)", resultsProd, respsRecv)
	}
	if resultsCons != resultsProd {
		t.Errorf("Lifecycle mismatch: Results Consumed (%d) != Results Produced (%d)", resultsCons, resultsProd)
	}
}
