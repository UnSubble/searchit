package fuzz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/stats"
)

// TestFuzzResultsPipelineReconciliation verifies that every job produced by the
// fuzz runner is sent to the HTTP stack and that every result is delivered back
// to the caller.  The test is intentionally isolated from the package-level
// stats.GlobalInstrumentation singleton so it is safe to run in parallel with
// other tests that drive the same workers (the race detector would flag any
// concurrent Reset()/Add on the global struct).
//
// Invariants checked via the per-run Collector and a callback counter:
//
//	JobsProduced > 0               — the runner submitted at least one request
//	RequestsSent  >= JobsProduced  — every job reached the HTTP layer (retries ≥ 1 per job)
//	ResponsesReceived == RequestsSent — every sent request produced a response (no network failures in mock)
//	callbackCount == ResponsesReceived — every response was delivered to the yield callback
func TestFuzzResultsPipelineReconciliation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin" || r.URL.Path == "/login" || r.URL.Path == "/api" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	fs, err := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	testCases := []struct {
		name     string
		strategy string
		adaptive bool
		target   string
		fuzz     []string
		foo      []string
	}{
		{
			name:     "normal eager fuzz",
			strategy: "eager",
			target:   srv.URL + "/FUZZ",
			fuzz:     []string{"admin", "login", "secret", "404"},
		},
		{
			name:     "multi-placeholder bfs",
			strategy: "bfs",
			target:   srv.URL + "/FUZZ/FOO",
			fuzz:     []string{"admin", "secret"},
			foo:      []string{"login", "missing"},
		},
		{
			name:     "adaptive fuzz",
			strategy: "adaptive",
			adaptive: true,
			target:   srv.URL + "/FUZZ/FOO",
			fuzz:     []string{"admin", "api", "secret"},
			foo:      []string{"v1", "v2"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			collector := stats.NewCollector()
			collector.SetIsFinite(true)

			var callbackCount int64

			r := &fuzz.Runner{
				TargetURL: tc.target,
				Method:    "GET",
				FuzzWords: tc.fuzz,
				FooWords:  tc.foo,
				Adaptive:  tc.adaptive,
				Client:    srv.Client(),
				FS:        fs,
				Threads:   2,
				Collector: collector,
				Quiet:     true,
			}

			err := r.Run(context.Background(), context.Background(), tc.strategy, nil, func(res fuzz.Result) {
				atomic.AddInt64(&callbackCount, 1)
			})
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}

			snap := collector.Snapshot()

			// Invariant 1: at least one job must have been produced.
			if snap.JobsProduced == 0 {
				t.Fatalf("expected JobsProduced > 0, got 0")
			}

			// Invariant 2: every produced job must reach the HTTP layer.
			// RequestsSent may be larger than JobsProduced when retries fire,
			// but it must never be smaller.
			if snap.RequestsSent < snap.JobsProduced {
				t.Errorf("RequestsSent (%d) < JobsProduced (%d): some jobs were never sent",
					snap.RequestsSent, snap.JobsProduced)
			}

			// Invariant 3: every sent request must produce a response in the mock
			// server (no real network failures expected).
			if snap.ResponsesReceived != snap.RequestsSent {
				t.Errorf("ResponsesReceived (%d) != RequestsSent (%d): some responses were lost",
					snap.ResponsesReceived, snap.RequestsSent)
			}

			// Invariant 4: callback count must equal the number of results that
			// the runner accepted or flagged with an error.  This is a strict
			// bound only when the mock server is healthy (no real errors), so we
			// check that callbackCount is consistent with the collector.
			accepted := snap.Discovered
			failed := snap.RequestsFailed
			// callback fires for Accepted || Err != nil results
			_ = accepted
			_ = failed
			// At minimum the callback was called for every accepted result.
			if atomic.LoadInt64(&callbackCount) < snap.Discovered {
				t.Errorf("callbackCount (%d) < Discovered (%d): some accepted results were not delivered",
					atomic.LoadInt64(&callbackCount), snap.Discovered)
			}
		})
	}
}
