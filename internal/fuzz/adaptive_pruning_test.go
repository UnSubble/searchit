package fuzz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/stats"
)

func TestAdaptivePruningCompletedAccounting(t *testing.T) {
	var requestedPaths []string
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestedPaths = append(requestedPaths, r.URL.Path)
		mu.Unlock()

		// Only /api and its subpaths return 200; everything else returns 404
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	fuzzWords := []string{"admin", "api", "secret", "user", "internal"} // 5 words
	barWords := []string{"v1", "v2", "v3", "v4"}                        // 4 words
	totalExpected := int64(len(fuzzWords) * len(barWords))              // 20 candidates

	collector := stats.NewCollector()
	collector.SetIsFinite(true)
	collector.SetTotalCandidates(totalExpected)

	r := &fuzz.Runner{
		TargetURL: srv.URL + "/FUZZ/BAR",
		Method:    "GET",
		FuzzWords: fuzzWords,
		BarWords:  barWords,
		Adaptive:  true,
		Client:    srv.Client(),
		FS:        fs,
		Threads:   2,
		Collector: collector,
	}

	var results []fuzz.Result
	err = r.Run(context.Background(), context.Background(), "adaptive", nil, func(res fuzz.Result) {
		if res.Accepted {
			results = append(results, res)
		}
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	snap := collector.Snapshot()

	if snap.TotalCandidates != totalExpected {
		t.Errorf("TotalCandidates: got %d, want %d", snap.TotalCandidates, totalExpected)
	}

	// 4 rejected prefixes * 4 leaves each = 16 skipped candidates
	expectedSkipped := int64(4 * 4)
	if snap.Skipped != expectedSkipped {
		t.Errorf("Skipped: got %d, want %d", snap.Skipped, expectedSkipped)
	}

	// 1 accepted prefix (/api) with 4 leaf requests executed = 4 tried candidates
	expectedTried := int64(4)
	if snap.Tried != expectedTried {
		t.Errorf("Tried: got %d, want %d", snap.Tried, expectedTried)
	}

	// Completed = Tried + Skipped == TotalCandidates (20)
	if snap.Completed != totalExpected {
		t.Errorf("Completed: got %d, want %d", snap.Completed, totalExpected)
	}

	if snap.Tried+snap.Skipped != totalExpected {
		t.Errorf("Tried (%d) + Skipped (%d) != TotalCandidates (%d)", snap.Tried, snap.Skipped, totalExpected)
	}

	if snap.Progress != 100.0 {
		t.Errorf("Progress: got %.1f%%, want 100.0%%", snap.Progress)
	}

	// Verify that successful branch (/api) explored all its descendants
	foundDescendants := 0
	for _, res := range results {
		if strings.Contains(res.URL, "/api/") {
			foundDescendants++
		}
	}
	if foundDescendants != len(barWords) {
		t.Errorf("expected %d descendants under /api, got %d", len(barWords), foundDescendants)
	}
}
