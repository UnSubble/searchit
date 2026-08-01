package fuzz

import (
	"context"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/httpclient"
	"github.com/unsubble/searchit/internal/stats"
	"golang.org/x/time/rate"
)

const bodyReadLimit = 4096
const bodyRegexLimit = 1024 * 1024

// WorkItem encapsulates a RequestDTO and an optional channel to send the Result to.
type WorkItem struct {
	Req   RequestDTO
	Reply chan<- Result
}

func sendResult(results chan<- Result, item WorkItem, res Result) {
	atomic.AddInt64(&stats.GlobalInstrumentation.ResultsProduced, 1)
	if item.Reply != nil {
		item.Reply <- res
	} else {
		results <- res
	}
}

func drainAndClose(body io.ReadCloser) int64 {
	if body == nil {
		return 0
	}
	n, _ := io.Copy(io.Discard, io.LimitReader(body, 2048))
	body.Close()
	return n
}

// Worker processes incoming fuzzed jobs from the channel.
func Worker(
	targetCtx context.Context,
	execCtx context.Context,
	client *http.Client,
	fs *filter.FilterSuite,
	delay time.Duration,
	limiter *rate.Limiter,
	jobs <-chan WorkItem,
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

	for item := range jobs {
		func() {
			replied := false
			defer func() {
				if !replied {
					sendResult(results, item, Result{
						URL:      item.Req.URL,
						Accepted: false,
						Err:      context.Canceled,
						UserData: item.Req.UserData,
					})
				}
			}()

			if targetCtx != nil && targetCtx.Err() != nil {
				return
			}
			if pauseBlocker != nil {
				if err := pauseBlocker(targetCtx); err != nil {
					return
				}
			}

			atomic.AddInt64(&stats.GlobalInstrumentation.WorkerJobsRecv, 1)
			if limiter != nil {
				err := limiter.Wait(targetCtx)
				if err != nil {
					atomic.AddInt64(&stats.GlobalInstrumentation.WorkerJobsRej, 1)
					return
				}
			}

			replied = process(targetCtx, execCtx, client, fs, item, results, collector)
			atomic.AddInt64(&stats.GlobalInstrumentation.WorkerJobsComp, 1)

			if delay > 0 {
				select {
				case <-targetCtx.Done():
					stats.GlobalInstrumentation.LogEvent("context cancellation")
					return
				case <-time.After(delay):
				}
			}
		}()
	}
}

func process(
	targetCtx context.Context,
	execCtx context.Context,
	client *http.Client,
	fs *filter.FilterSuite,
	item WorkItem,
	results chan<- Result,
	collector *stats.Collector,
) bool {
	if targetCtx != nil && targetCtx.Err() != nil {
		return false
	}

	var bodyReader io.Reader
	if len(item.Req.Body) > 0 {
		bodyReader = strings.NewReader(item.Req.Body)
	}

	reqCtx := execCtx
	if reqCtx == nil {
		reqCtx = targetCtx
	}
	if client != nil && client.Timeout > 0 {
		var reqCancel context.CancelFunc
		reqCtx, reqCancel = context.WithTimeout(reqCtx, client.Timeout)
		defer reqCancel()
	}

	req, err := http.NewRequestWithContext(reqCtx, item.Req.Method, item.Req.URL, bodyReader)
	if err != nil {
		if collector != nil {
			collector.RecordRequestFailed()
			if !item.Req.IsProbing {
				collector.RecordSkipped(1)
			}
		}
		sendResult(results, item, Result{
			URL:      item.Req.URL,
			Accepted: false,
			Err:      err,
			UserData: item.Req.UserData,
		})
		return true
	}

	for k, values := range item.Req.Headers {
		if strings.EqualFold(k, "Host") && len(values) > 0 {
			req.Host = values[0]
			continue
		}
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}

	for _, c := range item.Req.Cookies {
		req.Header.Add("Cookie", c)
	}

	atomic.AddInt64(&stats.GlobalInstrumentation.RequestsBuilt, 1)
	var resp *http.Response
	maxRetries := 3
	var startTime time.Time
	for i := 0; i < maxRetries; i++ {
		if req.GetBody != nil {
			newBody, err := req.GetBody()
			if err == nil {
				req.Body = newBody
			}
		}

		startTime = time.Now()
		atomic.AddInt64(&stats.GlobalInstrumentation.RequestsSent, 1)
		if collector != nil {
			collector.RecordRequestSent()
			if i == 0 && !item.Req.IsProbing {
				collector.RecordTried()
			} else if i > 0 {
				collector.RecordRetry()
			}
		}
		resp, err = client.Do(req)
		if err == nil {
			if collector != nil {
				collector.RecordLatency(time.Since(startTime))
			}
			break
		}
		// Check context before retrying
		if targetCtx != nil && targetCtx.Err() != nil {
			err = targetCtx.Err()
			break
		}
		time.Sleep(10 * time.Millisecond * time.Duration(i+1))
	}

	if err != nil {
		if collector != nil {
			collector.RecordRequestFailed()
		}
		sendResult(results, item, Result{
			URL:      item.Req.URL,
			Accepted: false,
			Err:      err,
			UserData: item.Req.UserData,
		})
		return true
	}
	atomic.AddInt64(&stats.GlobalInstrumentation.ResponsesReceived, 1)

	if collector != nil {
		collector.RecordLatency(time.Since(startTime))
	}

	contentType := resp.Header.Get("Content-Type")
	length := httpclient.ContentLength(resp)

	// Filter 1: Match Headers (Status, Content-Type, Size)
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
		sendResult(results, item, Result{
			URL:        item.Req.URL,
			StatusCode: resp.StatusCode,
			Length:     length,
			Accepted:   false,
			UserData:   item.Req.UserData,
		})
		return true
	}

	// Filter 2: Match Body (Regex)
	var bodyBytes []byte
	bodyRead := false
	var readErr error
	if fs.RequiresBody() {
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
	} else {
		// Fast path drainage to keep connection alive
		extra, err := io.Copy(io.Discard, io.LimitReader(resp.Body, bodyReadLimit))
		if err == nil && extra == bodyReadLimit {
			more, _ := io.Copy(io.Discard, resp.Body)
			extra += more
		}
		resp.Body.Close()
		if length == -1 && err == nil {
			length = extra
		}
		if err != nil {
			if collector != nil {
				recLen := length
				if recLen < 0 {
					recLen = 0
				}
				collector.RecordResponseReceived(resp.StatusCode, recLen)
				collector.RecordRequestSucceeded()
				collector.RecordDiscovered()
			}
			sendResult(results, item, Result{
				URL:        item.Req.URL,
				StatusCode: resp.StatusCode,
				Length:     length,
				Accepted:   true,
				Err:        err,
				UserData:   item.Req.UserData,
			})
			return true
		}
	}

	// Late Size Filter Evaluation
	recLen := length
	if recLen < 0 {
		recLen = 0
	}

	if length != -1 && ((len(fs.MatchSize) > 0 && !fs.MatchSize.Match(length)) || (len(fs.FilterSize) > 0 && fs.FilterSize.Match(length))) {
		if collector != nil {
			collector.RecordResponseReceived(resp.StatusCode, recLen)
			collector.RecordRequestFiltered()
		}
		sendResult(results, item, Result{
			URL:        item.Req.URL,
			StatusCode: resp.StatusCode,
			Length:     length,
			Accepted:   false,
			Err:        readErr,
			UserData:   item.Req.UserData,
		})
		return true
	}

	if bodyRead {
		if readErr != nil || !fs.MatchBody(bodyBytes) {
			if collector != nil {
				collector.RecordResponseReceived(resp.StatusCode, recLen)
				collector.RecordRequestFiltered()
			}
			sendResult(results, item, Result{
				URL:        item.Req.URL,
				StatusCode: resp.StatusCode,
				Length:     length,
				Accepted:   false,
				Err:        readErr,
				UserData:   item.Req.UserData,
			})
			return true
		}
	}

	var resHeaders http.Header
	if fs.ShowHeaders {
		resHeaders = resp.Header
	} else if loc := resp.Header.Get("Location"); loc != "" {
		resHeaders = make(http.Header)
		resHeaders.Set("Location", loc)
	}

	var title string
	if fs.ShowTitle && bodyRead && readErr == nil {
		title = extractHTMLTitle(bodyBytes)
	}

	// Capture redirect destination for display (same-host only, like scan engine).
	var redirectURL string
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		if loc != "" {
			if u, err := url.Parse(loc); err == nil && resp.Request != nil && resp.Request.URL != nil {
				resolved := resp.Request.URL.ResolveReference(u)
				if resolved.Host == resp.Request.URL.Host {
					redirectURL = resolved.String()
				}
			}
		}
	}

	if collector != nil {
		collector.RecordResponseReceived(resp.StatusCode, recLen)
		collector.RecordRequestSucceeded()
		collector.RecordDiscovered()
	}

	sendResult(results, item, Result{
		URL:         item.Req.URL,
		RedirectURL: redirectURL,
		StatusCode:  resp.StatusCode,
		Length:      length,
		Accepted:    true,
		Title:       title,
		Headers:     resHeaders,
		UserData:    item.Req.UserData,
	})
	return true
}

var titleRx = regexp.MustCompile(`(?i)<title(?:\s+[^>]*)?>([^<]*)</title>`)

func extractHTMLTitle(body []byte) string {
	m := titleRx.FindSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(string(m[1])))
}
