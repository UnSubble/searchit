package recursion_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
)

func TestGoldenCorrectnessAndWorkerConsistency(t *testing.T) {
	// 1. Predefined expected existing routes
	expectedExisting := map[string]int{
		"/static":        200,
		"/health":        200,
		"/admin":         200,
		"/admin/panel":   200,
		"/redirect":      302,
		"/query?a=1":     200,
		"/redirect/dest": 200,
	}

	// Setup mock server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Root"))
			return
		}

		pathWithQuery := r.URL.Path
		if r.URL.RawQuery != "" {
			pathWithQuery += "?" + r.URL.RawQuery
		}

		if statusVal, ok := expectedExisting[pathWithQuery]; ok {
			if statusVal == 302 {
				w.Header().Set("Location", "/redirect/dest")
			}
			w.WriteHeader(statusVal)
			_, _ = w.Write([]byte("OK"))
			return
		}

		if statusVal, ok := expectedExisting[r.URL.Path]; ok {
			w.WriteHeader(statusVal)
			_, _ = w.Write([]byte("OK"))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	// 2. Define wordlist candidates, containing duplicates, fragments, dot segments, whitespace, etc.
	reader := staticReader{
		words: []string{
			"static",
			"health",
			"  admin  ",     // whitespace surrounding
			"admin",         // duplicate
			"admin#section", // fragment to be stripped (resolves to duplicate "admin")
			"admin/panel",   // sub-segment
			"redirect",
			"query?a=1",      // explicit query string
			"#fragment-only", // fragment-only, should be rejected
			".",              // dot segment, should be rejected
			"..",             // double dot segment, should be rejected
		},
	}

	// Expected discoveries (canonical URLs stripped of fragments)
	expectedURLs := map[string]struct{}{
		srv.URL:                  {},
		srv.URL + "/static":      {},
		srv.URL + "/health":      {},
		srv.URL + "/admin":       {},
		srv.URL + "/admin/panel": {},
		srv.URL + "/redirect":    {},
		srv.URL + "/query?a=1":   {},
	}

	workerCounts := []int{1, 2, 4, 8, 16, 32, 64, 128, 256}

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
				2, // maxDepth 2 to allow /admin/panel and /redirect/dest
				recurseOnFilters,
				true, // normalizePaths
				true, // collapseSlashes
				nil, nil, nil, nil, 0, nil, nil,
			)

			ctx := context.Background()
			resultsChan := manager.Run(ctx, []string{srv.URL}, wc)

			// Collect actual discoveries
			actualDiscoveries := make(map[string]int)
			var mu sync.Mutex
			for r := range resultsChan {
				mu.Lock()
				actualDiscoveries[r.URL]++
				mu.Unlock()
			}

			// Perform Golden Comparison
			missingCount := 0
			unexpectedCount := 0
			duplicateCount := 0

			// 1. Check for missing
			for expected := range expectedURLs {
				if _, ok := actualDiscoveries[expected]; !ok {
					missingCount++
					t.Errorf("Missing expected discovery: %s", expected)
				}
			}

			// 2. Check for unexpected and duplicates
			for actual, count := range actualDiscoveries {
				// Strip client-side fragment just in case
				u, err := url.Parse(actual)
				if err != nil {
					t.Errorf("Malformed actual URL: %s", actual)
					continue
				}
				if u.Fragment != "" {
					t.Errorf("Incorrect behavior: printed URL has fragment component: %s", actual)
				}

				if _, ok := expectedURLs[actual]; !ok {
					unexpectedCount++
					t.Errorf("Unexpected discovery: %s", actual)
				}
				if count > 1 {
					duplicateCount += (count - 1)
					t.Errorf("Duplicate discovery: %s was reported %d times", actual, count)
				}
			}

			statusStr := "PASS"
			if missingCount > 0 || unexpectedCount > 0 || duplicateCount > 0 {
				statusStr = "FAIL"
			}

			t.Logf("Expected Discoveries : %d", len(expectedURLs))
			t.Logf("Actual Discoveries   : %d", len(actualDiscoveries))
			t.Logf("Missing              : %d", missingCount)
			t.Logf("Unexpected           : %d", unexpectedCount)
			t.Logf("Duplicates           : %d", duplicateCount)
			t.Logf("Status               : %s", statusStr)

			if statusStr != "PASS" {
				t.Fatalf("Golden correctness verification failed under %d workers", wc)
			}

			// Verify HTTP Lifecycle Invariant
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
		})
	}
}
