package engine

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/stats"
	"golang.org/x/time/rate"
)

// Scanner is the public orchestration entry point.
// Workers and pool management are internal details.
type Scanner struct {
	client     *http.Client
	fs         *filter.FilterSuite
	incHeaders []HeaderFilter
	excHeaders []HeaderFilter
	errors     chan error
	delay      time.Duration
	limiter    *rate.Limiter
	stats      *stats.Collector

	// Request manipulation fields
	method  string
	body    []byte
	headers http.Header
	cookies []*http.Cookie
}

// SetRequestManipulation configures outbound fuzzed request templates for scanning.
func (s *Scanner) SetRequestManipulation(method string, body []byte, headers http.Header, cookies []*http.Cookie) {
	s.method = method
	s.body = body
	s.headers = headers
	s.cookies = cookies
}

func NewScanner(
	client *http.Client,
	fs *filter.FilterSuite,
	incHeaders, excHeaders []HeaderFilter,
	delay time.Duration,
	limiter *rate.Limiter,
) *Scanner {
	return &Scanner{
		client:     client,
		fs:         fs,
		incHeaders: incHeaders,
		excHeaders: excHeaders,
		errors:     make(chan error, 1),
		delay:      delay,
		limiter:    limiter,
	}
}

// SetStats sets the statistics collector for the scanner.
func (s *Scanner) SetStats(c *stats.Collector) {
	s.stats = c
}

// Scan starts the producer and a worker pool, returning a results channel that
// is closed when the scan completes.
// Cancelling ctx stops job emission and aborts in-flight requests.
func (s *Scanner) Scan(ctx context.Context, producer Producer, workers int) <-chan Result {
	jobs := make(chan Job, workers)
	results := Start(ctx, s.client, s.fs, s.incHeaders, s.excHeaders, workers, s.delay, s.limiter, s.method, s.body, s.headers, s.cookies, jobs, s.stats)
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
			atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
			if r.Accepted {
				atomic.AddInt64(&stats.GlobalInstrumentation.ResultsAccepted, 1)
				out <- r
			} else {
				atomic.AddInt64(&stats.GlobalInstrumentation.ResultsRejected, 1)
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
