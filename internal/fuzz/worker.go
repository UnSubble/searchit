package fuzz

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/httpclient"
	"github.com/unsubble/searchit/internal/stats"
	"golang.org/x/time/rate"
)

const bodyReadLimit = 4096
const bodyRegexLimit = 1024 * 1024

func sendResult(results chan<- Result, res Result) {
	atomic.AddInt64(&stats.GlobalInstrumentation.ResultsProduced, 1)
	results <- res
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 2048))
	body.Close()
}

// Worker processes incoming fuzzed jobs from the channel.
func Worker(
	ctx context.Context,
	client *http.Client,
	fs *filter.FilterSuite,
	delay time.Duration,
	limiter *rate.Limiter,
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

		process(ctx, client, fs, job, results, collector)
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
	fs *filter.FilterSuite,
	job Job,
	results chan<- Result,
	collector *stats.Collector,
) {
	if collector != nil {
		collector.RecordRequestSent()
	}

	var bodyReader io.Reader
	if len(job.Body) > 0 {
		bodyReader = bytes.NewReader(job.Body)
	}

	req, err := http.NewRequestWithContext(ctx, job.Method, job.URL, bodyReader)
	if err != nil {
		if collector != nil {
			collector.RecordRequestFailed()
		}
		sendResult(results, Result{
			URL:      job.URL,
			Accepted: false,
			Err:      err,
		})
		return
	}

	for k, values := range job.Headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}

	for _, c := range job.Cookies {
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
			Accepted: false,
			Err:      err,
		})
		return
	}
	atomic.AddInt64(&stats.GlobalInstrumentation.ResponsesReceived, 1)

	if collector != nil {
		collector.RecordLatency(time.Since(startTime))
	}

	contentType := resp.Header.Get("Content-Type")
	length := httpclient.ContentLength(resp)

	// Filter 1: Match Headers (Status, Content-Type, Size)
	if !fs.MatchHeaders(resp.StatusCode, length, contentType) {
		drainAndClose(resp.Body)
		if collector != nil {
			collector.RecordResponseReceived(resp.StatusCode, length)
			collector.RecordRequestFiltered()
		}
		sendResult(results, Result{
			URL:        job.URL,
			StatusCode: resp.StatusCode,
			Length:     length,
			Accepted:   false,
		})
		return
	}

	// Filter 2: Match Body (Regex)
	var bodyBytes []byte
	bodyRead := false
	var readErr error
	if fs.RequiresBody() {
		bodyBytes, readErr = io.ReadAll(io.LimitReader(resp.Body, bodyRegexLimit))
		bodyRead = true
		resp.Body.Close()
	}

	if bodyRead {
		if readErr != nil || !fs.MatchBody(bodyBytes) {
			if collector != nil {
				collector.RecordResponseReceived(resp.StatusCode, length)
				collector.RecordRequestFiltered()
			}
			sendResult(results, Result{
				URL:        job.URL,
				StatusCode: resp.StatusCode,
				Length:     length,
				Accepted:   false,
				Err:        readErr,
			})
			return
		}
	} else {
		// Fast path drainage to keep connection alive
		if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, bodyReadLimit)); err != nil {
			resp.Body.Close()
			if collector != nil {
				collector.RecordResponseReceived(resp.StatusCode, length)
				collector.RecordRequestSucceeded()
			}
			sendResult(results, Result{
				URL:        job.URL,
				StatusCode: resp.StatusCode,
				Length:     length,
				Accepted:   true,
				Err:        err,
			})
			return
		}
		resp.Body.Close()
	}

	if collector != nil {
		collector.RecordResponseReceived(resp.StatusCode, length)
		collector.RecordRequestSucceeded()
	}

	sendResult(results, Result{
		URL:        job.URL,
		StatusCode: resp.StatusCode,
		Length:     length,
		Accepted:   true,
	})
}
