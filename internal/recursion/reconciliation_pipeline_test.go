package recursion_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/stats"
)

func TestRecursiveScanPipelineReconciliation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "", "/", "/sub":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	stats.GlobalInstrumentation.Reset()
	atomic.StoreInt32(&stats.GlobalInstrumentation.Enabled, 1)
	defer stats.GlobalInstrumentation.Reset()

	a := app.New(context.Background(), config.Default())
	reader := staticReader{words: []string{"sub", "other", "test"}}

	mgr := recursion.NewManager(
		a.HTTPClient,
		nil,
		reader,
		recursion.BFS,
		2,
		a.Config.RecurseOn,
		false,
		false,
		0,
		nil,
		nil,
		3,
	)

	collector := stats.NewCollector()
	collector.SetIsFinite(false)
	mgr.SetStats(collector)

	ctx := context.Background()
	_ = collectResults(mgr, ctx, []string{ts.URL}, 2)

	jobsProd := atomic.LoadInt64(&stats.GlobalInstrumentation.JobsProduced)
	jobsSub := atomic.LoadInt64(&stats.GlobalInstrumentation.JobsSubmitted)
	jobsRecv := atomic.LoadInt64(&stats.GlobalInstrumentation.WorkerJobsRecv)
	resProd := atomic.LoadInt64(&stats.GlobalInstrumentation.ResultsProduced)
	resCons := atomic.LoadInt64(&stats.GlobalInstrumentation.ResultsConsumed)

	if jobsProd == 0 {
		t.Fatalf("expected JobsProduced > 0, got 0")
	}
	if jobsProd != jobsSub {
		t.Fatalf("pipeline mismatch: JobsProduced (%d) != JobsSubmitted (%d)", jobsProd, jobsSub)
	}
	if jobsSub != jobsRecv {
		t.Fatalf("pipeline mismatch: JobsSubmitted (%d) != JobsReceived (%d)", jobsSub, jobsRecv)
	}
	if resProd != resCons {
		t.Fatalf("pipeline mismatch: ResultsProduced (%d) != ResultsConsumed (%d)", resProd, resCons)
	}

	var buf bytes.Buffer
	stats.GlobalInstrumentation.PrintReconciliation(&buf)
	reconciliationOut := buf.String()

	if bytes.Contains(buf.Bytes(), []byte("MISMATCH DETECTED")) {
		t.Fatalf("expected pipeline reconciliation success, but got mismatch:\n%s", reconciliationOut)
	}
}
