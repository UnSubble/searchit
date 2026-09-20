package fuzz_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/stats"
)

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

	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
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
		t.Run(tc.name, func(t *testing.T) {
			stats.GlobalInstrumentation.Reset()
			atomic.StoreInt32(&stats.GlobalInstrumentation.Enabled, 1)
			defer stats.GlobalInstrumentation.Reset()

			collector := stats.NewCollector()
			collector.SetIsFinite(true)

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
			}

			err := r.Run(context.Background(), context.Background(), tc.strategy, nil, func(res fuzz.Result) {})
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}

			jobsProd := atomic.LoadInt64(&stats.GlobalInstrumentation.JobsProduced)
			jobsSub := atomic.LoadInt64(&stats.GlobalInstrumentation.JobsSubmitted)
			resProd := atomic.LoadInt64(&stats.GlobalInstrumentation.ResultsProduced)
			resCons := atomic.LoadInt64(&stats.GlobalInstrumentation.ResultsConsumed)

			if jobsProd == 0 {
				t.Fatalf("expected JobsProduced > 0, got 0")
			}
			if jobsProd != jobsSub {
				t.Errorf("JobsProduced (%d) != JobsSubmitted (%d)", jobsProd, jobsSub)
			}
			if resProd == 0 {
				t.Fatalf("expected ResultsProduced > 0, got 0")
			}
			if resProd != resCons {
				t.Errorf("ResultsProduced (%d) != ResultsConsumed (%d)", resProd, resCons)
			}

			var buf bytes.Buffer
			stats.GlobalInstrumentation.PrintReconciliation(&buf)
			if bytes.Contains(buf.Bytes(), []byte("MISMATCH DETECTED")) {
				t.Fatalf("pipeline mismatch detected:\n%s", buf.String())
			}
		})
	}
}
