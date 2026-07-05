package engine

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
)

// Start launches workers goroutines and returns a results channel that is
// closed once every worker exits. The caller must close jobs to signal
// completion and must drain results to avoid blocking workers.
func Start(
	ctx context.Context,
	client *http.Client,
	exclude status.Filters,
	incSize, excSize size.Filters,
	incHeaders, excHeaders []HeaderFilter,
	workers int,
	delay time.Duration,
	jobs <-chan Job,
) <-chan Result {
	results := make(chan Result, workers)

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			Worker(ctx, client, exclude, incSize, excSize, incHeaders, excHeaders, delay, jobs, results)
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}
