package fuzz

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/stats"
	"golang.org/x/time/rate"
)

// Start launches worker goroutines and returns a results channel that is
// closed once every worker exits. The caller must close the jobs channel to
// signal completion and must drain results to avoid blocking workers.
func Start(
	ctx context.Context,
	client *http.Client,
	fs *filter.FilterSuite,
	workers int,
	delay time.Duration,
	limiter *rate.Limiter,
	jobs <-chan Job,
	collector *stats.Collector,
) <-chan Result {
	results := make(chan Result, workers)

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			Worker(ctx, client, fs, delay, limiter, jobs, results, collector)
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}
