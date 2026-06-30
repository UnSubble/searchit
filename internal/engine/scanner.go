package engine

import (
	"context"

	"github.com/unsubble/searchit/internal/app"
)

// Scanner is the public orchestration entry point.
// Workers and pool management are internal details.
type Scanner struct {
	app    *app.App
	errors chan error
}

func NewScanner(a *app.App) *Scanner {
	return &Scanner{
		app:    a,
		errors: make(chan error, 1),
	}
}

// Scan starts the producer and a worker pool, returning a results channel that
// is closed when the scan completes.
// Cancelling ctx stops job emission and aborts in-flight requests.
func (s *Scanner) Scan(ctx context.Context, producer Producer, workers int) <-chan Result {
	jobs := make(chan Job, workers)
	results := Start(ctx, s.app, workers, jobs)

	go func() {
		if err := producer.Produce(ctx, jobs); err != nil && ctx.Err() == nil {
			// Only forward non-cancellation errors; context cancellation is
			// expected and not treated as a failure.
			select {
			case s.errors <- err:
			default:
			}
		}
	}()

	return results
}

// Err returns the first producer error encountered during the last scan, if any.
// Call after draining the results channel.
func (s *Scanner) Err() error {
	select {
	case err := <-s.errors:
		return err
	default:
		return nil
	}
}
