package engine

import (
	"context"

	"github.com/unsubble/searchit/internal/app"
)

// Scanner is the public orchestration entry point.
// Workers and pool management are internal details.
type Scanner struct {
	app *app.App
}

func NewScanner(a *app.App) *Scanner {
	return &Scanner{app: a}
}

// Scan starts the producer and a worker pool, returning a results channel that
// is closed when the scan completes.
// Cancelling ctx stops job emission and aborts in-flight requests.
func (s *Scanner) Scan(ctx context.Context, producer Producer, workers int) <-chan Result {
	jobs := make(chan Job, workers)
	results := Start(ctx, s.app, workers, jobs)

	go func() {
		// TODO: Surface producer errors through a dedicated error channel.
		_ = producer.Produce(ctx, jobs)
	}()

	return results
}
