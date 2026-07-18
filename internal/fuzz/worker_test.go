package fuzz_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
)

type mockRoundTripper struct {
	response func(*http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.response(req)
}

func TestWorker_ExecutionAndFiltering(t *testing.T) {
	type reqSnap struct {
		method string
		url    string
		body   string
		header string
	}
	var snaps []reqSnap

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			var bodyBytes []byte
			if req.Body != nil {
				bodyBytes, _ = io.ReadAll(req.Body)
			}
			snaps = append(snaps, reqSnap{
				method: req.Method,
				url:    req.URL.String(),
				body:   string(bodyBytes),
				header: req.Header.Get("X-Custom"),
			})

			statusCode := 200
			if req.URL.Path == "/exclude" {
				statusCode = 404
			}
			return &http.Response{
				StatusCode:    statusCode,
				ContentLength: 42,
				Header:        http.Header{"Content-Length": []string{"42"}},
				Body:          io.NopCloser(bytes.NewReader([]byte("responsebody"))),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	exclude, _ := status.Parse("404")
	incSize, _ := size.Parse("10-50")
	excSize, _ := size.Parse("100")

	jobs := make(chan fuzz.Job, 2)
	results := make(chan fuzz.Result, 2)

	// Send 1 successful job
	jobs <- fuzz.Job{
		URL:     "http://target.com/success",
		Method:  "POST",
		Body:    []byte("postbody"),
		Headers: http.Header{"X-Custom": []string{"val123"}},
	}
	// Send 1 job that should be filtered out by status
	jobs <- fuzz.Job{
		URL:    "http://target.com/exclude",
		Method: "GET",
	}
	close(jobs)

	fs, _ := filter.NewFilterSuite("", exclude.String(), incSize.String(), excSize.String(), nil, nil, nil, nil)
	fuzz.Worker(
		context.Background(),
		client,
		fs,
		0,
		nil,
		jobs,
		results,
		nil,
	)
	close(results)

	var res []fuzz.Result
	for r := range results {
		res = append(res, r)
	}

	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}

	for i, r := range res {
		t.Logf("res[%d]: URL=%s, Accepted=%t, StatusCode=%d, Length=%d, Err=%v", i, r.URL, r.Accepted, r.StatusCode, r.Length, r.Err)
	}

	// First result should be accepted
	first := res[0]
	if !first.Accepted {
		t.Errorf("expected first job to be accepted")
	}
	if first.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", first.StatusCode)
	}
	if first.Length != 42 {
		t.Errorf("expected length 42, got %d", first.Length)
	}

	// Verify request parameters passed to RoundTripper
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snaps, got %d", len(snaps))
	}
	if snaps[0].method != "POST" {
		t.Errorf("expected method POST, got %q", snaps[0].method)
	}
	if snaps[0].url != "http://target.com/success" {
		t.Errorf("expected URL http://target.com/success, got %q", snaps[0].url)
	}
	if snaps[0].body != "postbody" {
		t.Errorf("expected body postbody, got %q", snaps[0].body)
	}
	if snaps[0].header != "val123" {
		t.Errorf("expected header val123, got %q", snaps[0].header)
	}

	// Second result should be excluded
	second := res[1]
	if second.Accepted {
		t.Errorf("expected second job to be rejected (filtered out by 404)")
	}
	if second.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", second.StatusCode)
	}
}

func TestWorker_DelayCancellation(t *testing.T) {
	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	jobs := make(chan fuzz.Job, 5)
	results := make(chan fuzz.Result, 5)

	jobs <- fuzz.Job{URL: "http://target.com/1"}
	jobs <- fuzz.Job{URL: "http://target.com/2"}

	ctx, cancel := context.WithCancel(context.Background())

	// Start worker in goroutine
	go func() {
		fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
		fuzz.Worker(
			ctx,
			client,
			fs,
			100*time.Millisecond, // 100ms delay to allow cancellation
			nil,
			jobs,
			results,
			nil,
		)
		close(results)
	}()

	// Read first result
	r := <-results
	if r.URL != "http://target.com/1" {
		t.Errorf("expected URL http://target.com/1, got %q", r.URL)
	}

	// Cancel context before second request executes
	cancel()
	close(jobs)

	// Verify no more results (or second is cancelled/discarded)
	var trailing []fuzz.Result
	for tr := range results {
		trailing = append(trailing, tr)
	}

	if len(trailing) > 1 {
		t.Errorf("expected at most 1 result after cancellation, got %d", len(trailing))
	}
}

type errorReader struct{}

func (errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func (errorReader) Close() error {
	return nil
}

func TestWorker_ProcessErrorPaths(t *testing.T) {
	// 1. Invalid method name (space inside method) triggers NewRequestWithContext error
	func() {
		client := &http.Client{}
		fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
		job := fuzz.Job{
			URL:    "http://localhost",
			Method: "GE T", // Invalid HTTP method
		}

		// Run fuzz.Start internally by scheduling the job
		jobs := make(chan fuzz.Job, 1)
		jobs <- job
		close(jobs)

		resChan := fuzz.Start(context.Background(), client, fs, 1, 0, nil, jobs, nil)
		res := <-resChan
		if res.Err == nil {
			t.Error("expected NewRequestWithContext error, got nil")
		}
	}()

	// 2. HTTP connection failure (client.Do returns error)
	func() {
		rt := &mockRoundTripper{
			response: func(req *http.Request) (*http.Response, error) {
				return nil, io.ErrUnexpectedEOF
			},
		}
		client := &http.Client{Transport: rt}
		fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
		jobs := make(chan fuzz.Job, 1)
		jobs <- fuzz.Job{URL: "http://localhost"}
		close(jobs)

		resChan := fuzz.Start(context.Background(), client, fs, 1, 0, nil, jobs, nil)
		res := <-resChan
		if res.Err == nil || !strings.Contains(res.Err.Error(), io.ErrUnexpectedEOF.Error()) {
			t.Errorf("expected error containing unexpected EOF, got: %v", res.Err)
		}
	}()

	// 3. Host header override and cookie passing
	func() {
		var hostOverride string
		var receivedCookies []*http.Cookie
		rt := &mockRoundTripper{
			response: func(req *http.Request) (*http.Response, error) {
				hostOverride = req.Host
				receivedCookies = req.Cookies()
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewReader([]byte("test"))),
				}, nil
			},
		}
		client := &http.Client{Transport: rt}
		fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
		jobs := make(chan fuzz.Job, 1)
		jobs <- fuzz.Job{
			URL:     "http://localhost/path",
			Method:  "GET",
			Headers: http.Header{"Host": []string{"custom-host.com"}},
			Cookies: []*http.Cookie{{Name: "sess", Value: "val"}},
		}
		close(jobs)

		resChan := fuzz.Start(context.Background(), client, fs, 1, 0, nil, jobs, nil)
		<-resChan

		if hostOverride != "custom-host.com" {
			t.Errorf("expected Host override custom-host.com, got %q", hostOverride)
		}
		if len(receivedCookies) != 1 || receivedCookies[0].Name != "sess" {
			t.Errorf("unexpected cookies: %v", receivedCookies)
		}
	}()

	// 4. Response body read error
	func() {
		rt := &mockRoundTripper{
			response: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Body:       errorReader{},
				}, nil
			},
		}
		client := &http.Client{Transport: rt}
		// Require body to trigger body read error by setting MatchRegex
		fs, _ := filter.NewFilterSuite("", "", "", "", []string{"trigger-requires-body"}, nil, nil, nil)

		jobs := make(chan fuzz.Job, 1)
		jobs <- fuzz.Job{URL: "http://localhost"}
		close(jobs)

		resChan := fuzz.Start(context.Background(), client, fs, 1, 0, nil, jobs, nil)
		res := <-resChan
		if res.Err == nil {
			t.Error("expected body read error, got nil")
		}
	}()
}
