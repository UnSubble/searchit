package engine

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/unsubble/searchit/internal/httpclient"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
	"golang.org/x/time/rate"
)

const bodyReadLimit = 4096

// HeaderFilter specifies an exact match rule on a case-insensitive header name.
type HeaderFilter struct {
	Name  string
	Value string
}

// Worker executes the response pipeline for incoming jobs.
// Pipeline: Status -> Headers -> Content-Length -> Body
func Worker(
	ctx context.Context,
	client *http.Client,
	exclude status.Filters,
	incSize, excSize size.Filters,
	incHeaders, excHeaders []HeaderFilter,
	delay time.Duration,
	limiter *rate.Limiter,
	jobs <-chan Job,
	results chan<- Result,
	collector *stats.Collector,
) {
	if collector != nil {
		collector.IncrementActiveWorkers()
		defer collector.DecrementActiveWorkers()
	}
	for job := range jobs {
		if limiter != nil {
			err := limiter.Wait(ctx)
			if err != nil {
				return
			}
		}

		process(ctx, client, exclude, incSize, excSize, incHeaders, excHeaders, job, results, collector)

		if delay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
	}
}

func process(
	ctx context.Context,
	client *http.Client,
	exclude status.Filters,
	incSize, excSize size.Filters,
	incHeaders, excHeaders []HeaderFilter,
	job Job,
	results chan<- Result,
	collector *stats.Collector,
) {
	if collector != nil {
		collector.RecordRequestSent()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, job.URL, nil)
	if err != nil {
		if collector != nil {
			collector.RecordRequestFailed()
		}
		results <- Result{
			URL:      job.URL,
			Depth:    job.Depth,
			Accepted: false,
			Err:      err,
		}
		return
	}

	startTime := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		if collector != nil {
			collector.RecordRequestFailed()
		}
		results <- Result{
			URL:      job.URL,
			Depth:    job.Depth,
			Accepted: false,
			Err:      err,
		}
		return
	}

	if collector != nil {
		collector.RecordLatency(time.Since(startTime))
	}

	// Stage 1: Status
	if exclude.Match(resp.StatusCode) {
		resp.Body.Close()
		if collector != nil {
			collector.RecordResponseReceived(resp.StatusCode, 0)
			collector.RecordRequestFiltered()
		}
		results <- Result{
			URL:        job.URL,
			StatusCode: resp.StatusCode,
			Depth:      job.Depth,
			Accepted:   false,
		}
		return
	}

	// Stage 2: Headers
	if !AcceptHeaders(resp, incHeaders, excHeaders) {
		resp.Body.Close()
		if collector != nil {
			collector.RecordResponseReceived(resp.StatusCode, 0)
			collector.RecordRequestFiltered()
		}
		results <- Result{
			URL:        job.URL,
			StatusCode: resp.StatusCode,
			Depth:      job.Depth,
			Accepted:   false,
		}
		return
	}

	// Stage 3: Content-Length
	length := httpclient.ContentLength(resp)
	if !AcceptContentLength(length, incSize, excSize) {
		resp.Body.Close()
		if collector != nil {
			collector.RecordResponseReceived(resp.StatusCode, length)
			collector.RecordRequestFiltered()
		}
		results <- Result{
			URL:        job.URL,
			StatusCode: resp.StatusCode,
			Length:     length,
			Depth:      job.Depth,
			Accepted:   false,
		}
		return
	}

	// Stage 4: Body - read at most bodyReadLimit bytes.
	// Responses larger than this limit will not be fully drained, which
	// intentionally favors bounded memory usage over maximum keep-alive reuse.
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, bodyReadLimit)); err != nil {
		// Body read errors after passing all filters do not discard the result;
		// the status and headers were already validated. Close and continue.
		resp.Body.Close()
		if collector != nil {
			collector.RecordResponseReceived(resp.StatusCode, length)
			collector.RecordRequestSucceeded()
			collector.RecordDiscovered()
		}
		results <- Result{
			URL:        job.URL,
			StatusCode: resp.StatusCode,
			Length:     length,
			Depth:      job.Depth,
			Accepted:   true,
			Err:        err,
		}
		return
	}
	resp.Body.Close()

	if collector != nil {
		collector.RecordResponseReceived(resp.StatusCode, length)
		collector.RecordRequestSucceeded()
		collector.RecordDiscovered()
	}

	results <- Result{
		URL:        job.URL,
		StatusCode: resp.StatusCode,
		Length:     length,
		Depth:      job.Depth,
		Accepted:   true,
	}
}

// AcceptHeaders evaluates headers matching.
func AcceptHeaders(resp *http.Response, inc, exc []HeaderFilter) bool {
	for _, f := range exc {
		if matchHeader(resp, f.Name, f.Value) {
			return false
		}
	}
	for _, f := range inc {
		if !matchHeader(resp, f.Name, f.Value) {
			return false
		}
	}
	return true
}

func matchHeader(resp *http.Response, name, value string) bool {
	for k, values := range resp.Header {
		if strings.EqualFold(k, name) {
			for _, val := range values {
				if strings.EqualFold(val, value) {
					return true
				}
			}
		}
	}
	return false
}

// AcceptContentLength checks size constraints.
func AcceptContentLength(length int64, inc, exc size.Filters) bool {
	if exc.Match(length) {
		return false
	}
	if len(inc) > 0 && !inc.Match(length) {
		return false
	}
	return true
}
