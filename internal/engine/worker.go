package engine

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
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

func sendResult(results chan<- Result, res Result) {
	atomic.AddInt64(&stats.GlobalInstrumentation.ResultsProduced, 1)
	results <- res
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	// Limit read to 2048 bytes to discard small/typical bodies (like 404 responses),
	// allowing persistent TCP connection reuse without unbounded memory overhead.
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 2048))
	body.Close()
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
	method string,
	body []byte,
	headers http.Header,
	cookies []*http.Cookie,
	jobs <-chan Job,
	results chan<- Result,
	collector *stats.Collector,
) {
	atomic.AddInt64(&stats.GlobalInstrumentation.WorkersStarted, 1)
	defer func() {
		atomic.AddInt64(&stats.GlobalInstrumentation.WorkersExited, 1)
		stats.GlobalInstrumentation.LogEvent("worker exit")
	}()

	if collector != nil {
		collector.IncrementActiveWorkers()
		defer collector.DecrementActiveWorkers()
	}
	for job := range jobs {
		atomic.AddInt64(&stats.GlobalInstrumentation.WorkerJobsRecv, 1)
		if limiter != nil {
			err := limiter.Wait(ctx)
			if err != nil {
				atomic.AddInt64(&stats.GlobalInstrumentation.WorkerJobsRej, 1)
				return
			}
		}

		process(ctx, client, exclude, incSize, excSize, incHeaders, excHeaders, method, body, headers, cookies, job, results, collector)
		atomic.AddInt64(&stats.GlobalInstrumentation.WorkerJobsComp, 1)

		if delay > 0 {
			select {
			case <-ctx.Done():
				stats.GlobalInstrumentation.LogEvent("context cancellation")
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
	method string,
	body []byte,
	headers http.Header,
	cookies []*http.Cookie,
	job Job,
	results chan<- Result,
	collector *stats.Collector,
) {
	if collector != nil {
		collector.RecordRequestSent()
	}

	if method == "" {
		method = http.MethodGet
	}
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, job.URL, bodyReader)
	if err != nil {
		if collector != nil {
			collector.RecordRequestFailed()
		}
		sendResult(results, Result{
			URL:      job.URL,
			Depth:    job.Depth,
			Accepted: false,
			Origin:   job.Origin,
			Err:      err,
		})
		return
	}

	for k, values := range headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}

	atomic.AddInt64(&stats.GlobalInstrumentation.RequestsBuilt, 1)

	startTime := time.Now()
	atomic.AddInt64(&stats.GlobalInstrumentation.RequestsSent, 1)
	resp, err := client.Do(req)
	if err != nil {
		if collector != nil {
			collector.RecordRequestFailed()
		}
		sendResult(results, Result{
			URL:      job.URL,
			Depth:    job.Depth,
			Accepted: false,
			Origin:   job.Origin,
			Err:      err,
		})
		return
	}
	atomic.AddInt64(&stats.GlobalInstrumentation.ResponsesReceived, 1)

	if collector != nil {
		collector.RecordLatency(time.Since(startTime))
	}

	// Stage 1: Status
	if exclude.Match(resp.StatusCode) {
		drainAndClose(resp.Body)
		if collector != nil {
			collector.RecordResponseReceived(resp.StatusCode, 0)
			collector.RecordRequestFiltered()
		}
		sendResult(results, Result{
			URL:        job.URL,
			StatusCode: resp.StatusCode,
			Depth:      job.Depth,
			Accepted:   false,
			Origin:     job.Origin,
		})
		return
	}

	// Stage 2: Headers
	if !AcceptHeaders(resp, incHeaders, excHeaders) {
		drainAndClose(resp.Body)
		if collector != nil {
			collector.RecordResponseReceived(resp.StatusCode, 0)
			collector.RecordRequestFiltered()
		}
		sendResult(results, Result{
			URL:        job.URL,
			StatusCode: resp.StatusCode,
			Depth:      job.Depth,
			Accepted:   false,
			Origin:     job.Origin,
		})
		return
	}

	// Stage 3: Content-Length
	length := httpclient.ContentLength(resp)
	if !AcceptContentLength(length, incSize, excSize) {
		drainAndClose(resp.Body)
		if collector != nil {
			collector.RecordResponseReceived(resp.StatusCode, length)
			collector.RecordRequestFiltered()
		}
		sendResult(results, Result{
			URL:        job.URL,
			StatusCode: resp.StatusCode,
			Length:     length,
			Depth:      job.Depth,
			Accepted:   false,
			Origin:     job.Origin,
		})
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
		sendResult(results, Result{
			URL:        job.URL,
			StatusCode: resp.StatusCode,
			Length:     length,
			Depth:      job.Depth,
			Accepted:   true,
			Origin:     job.Origin,
			Err:        err,
		})
		return
	}
	resp.Body.Close()

	if collector != nil {
		collector.RecordResponseReceived(resp.StatusCode, length)
		collector.RecordRequestSucceeded()
		collector.RecordDiscovered()
	}

	sendResult(results, Result{
		URL:        job.URL,
		StatusCode: resp.StatusCode,
		Length:     length,
		Depth:      job.Depth,
		Accepted:   true,
		Origin:     job.Origin,
	})
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
