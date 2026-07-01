package engine

import (
	"context"
	"net/http"
	"sync"

	"github.com/unsubble/searchit/internal/status"
)

// Start launches workers goroutines and returns a results channel that is
// closed once every worker exits. The caller must close jobs to signal
// completion and must drain results to avoid blocking workers.
func Start(ctx context.Context, client *http.Client, exclude status.Filters, workers int, jobs <-chan Job) <-chan Result {
	results := make(chan Result, workers)

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			Worker(ctx, client, exclude, jobs, results)
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}
