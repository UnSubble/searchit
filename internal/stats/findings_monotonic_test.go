package stats_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
)

type staticSliceReader struct {
	words []string
}

func (r staticSliceReader) Read(ctx context.Context, out chan<- string) error {
	for _, w := range r.words {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- w:
		}
	}
	return nil
}

// TestFindingsMonotonic_RecursiveScan verifies that Findings (snap.Discovered)
// is strictly monotonic (never decreases) throughout a recursive scan containing
// a mixture of visible (200), filtered (404), and redirect (302) responses.
func TestFindingsMonotonic_RecursiveScan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "":
			w.WriteHeader(http.StatusOK) // 200 -> accepted finding
		case "/valid1", "/valid2":
			w.WriteHeader(http.StatusOK) // 200 -> accepted finding
		case "/missing1", "/missing2", "/missing3":
			w.WriteHeader(http.StatusNotFound) // 404 -> rejected by --fc 404
		default:
			w.WriteHeader(http.StatusForbidden) // 403 -> accepted finding
		}
	}))
	t.Cleanup(srv.Close)

	a := app.New(context.Background(), config.Default())

	m := recursion.NewManager(
		a.HTTPClient,
		nil,
		staticSliceReader{words: []string{"valid1", "missing1", "valid2", "missing2", "missing3"}},
		recursion.BFS,
		2,
		status.MustParse("200,301,302,403"),
		false,
		false,
		nil,
		nil,
		nil,
		nil,
		0,
		nil,
		nil,
		100,
	)

	displayFS, _ := filter.NewFilterSuite("", "404", "", "", nil, nil, nil, nil)
	m.SetFilterSuite(displayFS)

	collector := stats.NewCollector()
	m.SetStats(collector)

	// Monitor snap.Discovered continuously in a background goroutine
	// to detect any transient non-monotonic decreases or transient increments.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var lastDiscovered int64
	var nonMonotonicDetected int64
	var maxObservedDiscovered int64

	doneMon := make(chan struct{})
	go func() {
		defer close(doneMon)
		ticker := time.NewTicker(500 * time.Microsecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				snap := collector.Snapshot()
				cur := snap.Discovered

				if cur < atomic.LoadInt64(&lastDiscovered) {
					atomic.StoreInt64(&nonMonotonicDetected, 1)
				}
				if cur > atomic.LoadInt64(&maxObservedDiscovered) {
					atomic.StoreInt64(&maxObservedDiscovered, cur)
				}
				atomic.StoreInt64(&lastDiscovered, cur)
			}
		}
	}()

	var acceptedEmitted int64
	m.Run(context.Background(), context.Background(), []string{srv.URL + "/"}, 4, func(r engine.Result) {
		if r.Accepted && r.Err == nil {
			atomic.AddInt64(&acceptedEmitted, 1)
		}
	})

	cancel()
	<-doneMon

	snapFinal := collector.Snapshot()

	if atomic.LoadInt64(&nonMonotonicDetected) == 1 {
		t.Errorf("Findings counter was NOT monotonic: detected at least one decrement/correction during execution!")
	}

	if snapFinal.Discovered != acceptedEmitted {
		t.Errorf("snapFinal.Discovered (%d) != acceptedEmitted (%d)", snapFinal.Discovered, acceptedEmitted)
	}

	if atomic.LoadInt64(&maxObservedDiscovered) > acceptedEmitted {
		t.Errorf("maxObservedDiscovered (%d) exceeded acceptedEmitted (%d): transient findings leak detected!",
			atomic.LoadInt64(&maxObservedDiscovered), acceptedEmitted)
	}
}
