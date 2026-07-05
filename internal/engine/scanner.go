package engine

import (
	"context"
	"net/http"
	"time"

	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
)

// Scanner is the public orchestration entry point.
// Workers and pool management are internal details.
type Scanner struct {
	client     *http.Client
	exclude    status.Filters
	incSize    size.Filters
	excSize    size.Filters
	incHeaders []HeaderFilter
	excHeaders []HeaderFilter
	errors     chan error
	delay      time.Duration
}

func NewScanner(
	client *http.Client,
	exclude status.Filters,
	incSize, excSize size.Filters,
	incHeaders, excHeaders []HeaderFilter,
	delay time.Duration,
) *Scanner {
	return &Scanner{
		client:     client,
		exclude:    exclude,
		incSize:    incSize,
		excSize:    excSize,
		incHeaders: incHeaders,
		excHeaders: excHeaders,
		errors:     make(chan error, 1),
		delay:      delay,
	}
}

// Scan starts the producer and a worker pool, returning a results channel that
// is closed when the scan completes.
// Cancelling ctx stops job emission and aborts in-flight requests.
func (s *Scanner) Scan(ctx context.Context, producer Producer, workers int) <-chan Result {
	jobs := make(chan Job, workers)
	results := Start(ctx, s.client, s.exclude, s.incSize, s.excSize, s.incHeaders, s.excHeaders, workers, s.delay, jobs)
	out := make(chan Result, workers)

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

	go func() {
		defer close(out)
		for r := range results {
			if r.Accepted {
				out <- r
			}
		}
	}()

	return out
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
