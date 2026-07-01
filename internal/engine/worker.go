package engine

import (
	"context"
	"io"
	"net/http"

	"github.com/unsubble/searchit/internal/httpclient"
	"github.com/unsubble/searchit/internal/status"
)

const bodyReadLimit = 4096

// Worker executes the response pipeline for incoming jobs.
// Pipeline: Status -> Headers -> Content-Length -> Body
func Worker(ctx context.Context, client *http.Client, exclude status.Filters, jobs <-chan Job, results chan<- Result) {
	for job := range jobs {
		process(ctx, client, exclude, job, results)
	}
}

func process(ctx context.Context, client *http.Client, exclude status.Filters, job Job, results chan<- Result) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, job.URL, nil)
	if err != nil {
		results <- Result{
			URL:      job.URL,
			Depth:    job.Depth,
			Accepted: false,
			Err:      err,
		}
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		results <- Result{
			URL:      job.URL,
			Depth:    job.Depth,
			Accepted: false,
			Err:      err,
		}
		return
	}

	// Stage 1: Status
	if exclude.Match(resp.StatusCode) {
		resp.Body.Close()
		results <- Result{
			URL:        job.URL,
			StatusCode: resp.StatusCode,
			Depth:      job.Depth,
			Accepted:   false,
		}
		return
	}

	// Stage 2: Headers
	if !AcceptHeaders(resp) {
		resp.Body.Close()
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
	if !AcceptContentLength(length) {
		resp.Body.Close()
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

	results <- Result{
		URL:        job.URL,
		StatusCode: resp.StatusCode,
		Length:     length,
		Depth:      job.Depth,
		Accepted:   true,
	}
}

// AcceptHeaders is the header-filter hook.
// TODO: Implement content-type and baseline-header matching.
func AcceptHeaders(resp *http.Response) bool {
	return true
}

// AcceptContentLength is the content-length filter hook.
// TODO: Implement size-range and baseline-length filtering.
func AcceptContentLength(length int64) bool {
	return true
}
