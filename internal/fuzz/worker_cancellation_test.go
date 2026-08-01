package fuzz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/stats"
)

func TestWorkerProtocolCancellation(t *testing.T) {
	// A server that hangs, ensuring requests get queued and workers block.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)

	words := make([]string, 50)
	for i := 0; i < 50; i++ {
		words[i] = "test"
	}

	strategies := []string{"eager", "dfs", "bfs"}

	for _, strategy := range strategies {
		t.Run(strategy, func(t *testing.T) {
			r := &fuzz.Runner{
				TargetURL: srv.URL + "/FUZZ",
				Method:    "GET",
				FooWords:  words,
				Client:    srv.Client(),
				FS:        fs,
				Threads:   2,
				Collector: stats.NewCollector(),
			}

			ctx, cancel := context.WithCancel(context.Background())
			drainCtx := context.Background()

			go func() {
				// Cancel early, well before the workers can finish processing the 50 jobs
				time.Sleep(100 * time.Millisecond)
				cancel()
			}()

			errCh := make(chan error, 1)
			go func() {
				err := r.Run(ctx, drainCtx, strategy, nil, func(res fuzz.Result) {})
				errCh <- err
			}()

			select {
			case <-errCh:
				// Success, the function returned instead of deadlocking!
			case <-time.After(3 * time.Second):
				t.Fatalf("%s strategy deadlocked on cancellation", strategy)
			}
		})
	}
}
