package engine

import (
	"bytes"
	"context"
	"hash/fnv"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	htmlparser "github.com/unsubble/searchit/internal/html"
	"github.com/unsubble/searchit/internal/httpclient"
	"github.com/unsubble/searchit/internal/stats"
	"golang.org/x/time/rate"
)

const bodyRegexLimit = 1024 * 1024

// HeaderFilter specifies an exact match rule on a case-insensitive header name.
type HeaderFilter struct {
	Name  string
	Value string
}

func sendResult(results chan<- Result, res Result) {
	atomic.AddInt64(&stats.GlobalInstrumentation.ResultsProduced, 1)
	results <- res
}

func drainAndClose(body io.ReadCloser) int64 {
	if body == nil {
		return 0
	}
	// Limit read to 2048 bytes to discard small/typical bodies (like 404 responses),
	// allowing persistent TCP connection reuse without unbounded memory overhead.
	n, _ := io.Copy(io.Discard, io.LimitReader(body, 2048))
	body.Close()
	return n
}

// Worker executes the response pipeline for incoming jobs.
// Pipeline: Status -> Headers -> Content-Length -> Body
func Worker(
	targetCtx context.Context,
	execCtx context.Context,
	client *http.Client,
	fs *filter.FilterSuite,
	incHeaders, excHeaders []HeaderFilter,
	delay time.Duration,
	limiter *rate.Limiter,
	method string,
	body []byte,
	headers http.Header,
	cookieStr string,
	jobs <-chan Job,
	results chan<- Result,
	collector *stats.Collector,
	pauseBlocker func(context.Context) error,
) {
	if execCtx == nil {
		execCtx = targetCtx
	}
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
		if targetCtx != nil && targetCtx.Err() != nil {
			return
		}

		atomic.AddInt64(&stats.GlobalInstrumentation.WorkersActive, 1)
		if pauseBlocker != nil {
			if err := pauseBlocker(targetCtx); err != nil {
				atomic.AddInt64(&stats.GlobalInstrumentation.WorkersActive, -1)
				return
			}
		}

		atomic.AddInt64(&stats.GlobalInstrumentation.WorkerJobsRecv, 1)
		if limiter != nil {
			err := limiter.Wait(targetCtx)
			if err != nil {
				atomic.AddInt64(&stats.GlobalInstrumentation.WorkerJobsRej, 1)
				atomic.AddInt64(&stats.GlobalInstrumentation.WorkersActive, -1)
				return
			}
		}

		process(targetCtx, execCtx, client, fs, incHeaders, excHeaders, method, body, headers, cookieStr, job, results, collector)
		atomic.AddInt64(&stats.GlobalInstrumentation.WorkerJobsComp, 1)
		atomic.AddInt64(&stats.GlobalInstrumentation.WorkersActive, -1)

		if delay > 0 {
			select {
			case <-targetCtx.Done():
				stats.GlobalInstrumentation.LogEvent("context cancellation")
				return
			case <-time.After(delay):
			}
		}
	}
}

func process(
	targetCtx context.Context,
	execCtx context.Context,
	client *http.Client,
	fs *filter.FilterSuite,
	incHeaders, excHeaders []HeaderFilter,
	method string,
	body []byte,
	headers http.Header,
	cookieStr string,
	job Job,
	results chan<- Result,
	collector *stats.Collector,
) {
	if targetCtx != nil && targetCtx.Err() != nil {
		return
	}

	if method == "" {
		method = http.MethodGet
	}
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	reqCtx := execCtx
	if reqCtx == nil {
		reqCtx = targetCtx
	}

	req, err := http.NewRequestWithContext(reqCtx, method, job.URL, bodyReader)
	if err != nil {
		if collector != nil {
			collector.RecordRequestFailed()
			collector.RecordSkipped(1)
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
		if strings.EqualFold(k, "Host") && len(values) > 0 {
			req.Host = values[0]
			continue
		}
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	if cookieStr != "" {
		req.Header.Set("Cookie", cookieStr)
	}

	atomic.AddInt64(&stats.GlobalInstrumentation.RequestsBuilt, 1)

	startTime := time.Now()
	atomic.AddInt64(&stats.GlobalInstrumentation.RequestsSent, 1)
	if collector != nil {
		collector.RecordRequestSent()
		collector.RecordTried()
	}
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

	contentType := resp.Header.Get("Content-Type")
	length := httpclient.ContentLength(resp)

	// Stage 1: Match Headers (Status, Content-Type, Size)
	if !fs.MatchHeaders(resp.StatusCode, length, contentType) {
		drained := drainAndClose(resp.Body)
		if collector != nil {
			recLen := length
			if recLen < 0 {
				recLen = drained
			}
			collector.RecordResponseReceived(resp.StatusCode, recLen)
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

	// Stage 2: Headers (General Response HeaderFilter)
	if !AcceptHeaders(resp, incHeaders, excHeaders) {
		drained := drainAndClose(resp.Body)
		if collector != nil {
			recLen := length
			if recLen < 0 {
				recLen = drained
			}
			collector.RecordResponseReceived(resp.StatusCode, recLen)
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

	// Stage 3: Match Body (Regex)
	var bodyBytes []byte
	bodyRead := false
	var readErr error
	isHTML := strings.Contains(strings.ToLower(contentType), "text/html")
	// Always read body of responses passing header validation to extract links and compute hash
	bodyBytes, readErr = io.ReadAll(io.LimitReader(resp.Body, bodyRegexLimit))
	var extra int64
	if readErr == nil && len(bodyBytes) == int(bodyRegexLimit) {
		extra, _ = io.Copy(io.Discard, resp.Body)
	}
	bodyRead = true
	resp.Body.Close()

	if length == -1 && readErr == nil {
		length = int64(len(bodyBytes)) + extra
	}

	// Late Size Filter Evaluation
	recLen := length
	if recLen < 0 {
		recLen = int64(len(bodyBytes)) + extra
	}

	if length != -1 && ((len(fs.MatchSize) > 0 && !fs.MatchSize.Match(length)) || (len(fs.FilterSize) > 0 && fs.FilterSize.Match(length))) {
		if collector != nil {
			collector.RecordResponseReceived(resp.StatusCode, recLen)
			collector.RecordRequestFiltered()
		}
		sendResult(results, Result{
			URL:        job.URL,
			StatusCode: resp.StatusCode,
			Length:     length,
			Depth:      job.Depth,
			Accepted:   false,
			Origin:     job.Origin,
			Err:        readErr,
		})
		return
	}

	if readErr != nil || !fs.MatchBody(bodyBytes) {
		if collector != nil {
			collector.RecordResponseReceived(resp.StatusCode, recLen)
			collector.RecordRequestFiltered()
		}
		sendResult(results, Result{
			URL:        job.URL,
			StatusCode: resp.StatusCode,
			Length:     length,
			Depth:      job.Depth,
			Accepted:   false,
			Origin:     job.Origin,
			Err:        readErr,
		})
		return
	}

	var resHeaders http.Header
	if fs.ShowHeaders {
		resHeaders = resp.Header
	}

	var title string
	if fs.ShowTitle && bodyRead && readErr == nil {
		title = extractHTMLTitle(bodyBytes)
	}

	if collector != nil {
		collector.RecordResponseReceived(resp.StatusCode, recLen)
		collector.RecordRequestSucceeded()
		collector.RecordDiscovered()
	}

	var redirectURL string
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL := resp.Request.URL.String()
		if finalURL != job.URL {
			redirectURL = finalURL
		}
	}
	if redirectURL == "" && resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		if loc != "" {
			if u, err := url.Parse(loc); err == nil && resp.Request != nil && resp.Request.URL != nil {
				redirectURL = resp.Request.URL.ResolveReference(u).String()
			}
		}
	}
	if redirectURL != "" && resp.Request != nil && resp.Request.URL != nil {
		destURL, err2 := url.Parse(redirectURL)
		if err2 == nil {
			if resp.Request.URL.Host != destURL.Host {
				redirectURL = ""
			}
		} else {
			redirectURL = ""
		}
	}

	var links []string
	if isHTML && bodyRead && readErr == nil {
		links = htmlparser.ExtractLinks(bodyBytes)
	}

	var bodyHash uint64
	if bodyRead && readErr == nil {
		h := fnv.New64a()
		_, _ = h.Write(bodyBytes)
		bodyHash = h.Sum64()
	}

	sendResult(results, Result{
		URL:         job.URL,
		RedirectURL: redirectURL,
		StatusCode:  resp.StatusCode,
		Length:      length,
		Depth:       job.Depth,
		Accepted:    true,
		Origin:      job.Origin,
		Title:       title,
		Headers:     resHeaders,
		Links:       links,
		BodyHash:    bodyHash,
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

var titleRx = regexp.MustCompile(`(?i)<title(?:\s+[^>]*)?>([^<]*)</title>`)

func extractHTMLTitle(body []byte) string {
	m := titleRx.FindSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(string(m[1])))
}
