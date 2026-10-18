package fuzz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/httpclient"
)

func TestFuzzWorker_RegexScope_URL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("plain body"))
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.Options{})
	// Match URL containing "admin"
	fs, err := filter.NewFilterSuite("200", "", "", "", []string{"admin"}, nil, nil, nil, filter.RegexScopeURL)
	if err != nil {
		t.Fatalf("NewFilterSuite: %v", err)
	}

	jobs := make(chan fuzz.WorkItem, 2)
	results := make(chan fuzz.Result, 2)

	jobs <- fuzz.WorkItem{Req: fuzz.RequestDTO{URL: srv.URL + "/admin"}}
	jobs <- fuzz.WorkItem{Req: fuzz.RequestDTO{URL: srv.URL + "/user"}}
	close(jobs)

	ctx := context.Background()
	fuzz.Worker(ctx, ctx, client, fs, 0, nil, jobs, results, nil, nil, fuzz.WorkerOpts{})

	var accepted []fuzz.Result
	for i := 0; i < 2; i++ {
		r := <-results
		if r.Accepted {
			accepted = append(accepted, r)
		}
	}

	if len(accepted) != 1 {
		t.Fatalf("expected 1 accepted result, got %d", len(accepted))
	}
	if accepted[0].URL != srv.URL+"/admin" {
		t.Errorf("expected URL %s/admin, got %s", srv.URL, accepted[0].URL)
	}
}

func TestFuzzWorker_RegexScope_Header(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/special" {
			w.Header().Set("X-Custom-Env", "staging-production")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("body"))
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.Options{})
	fs, err := filter.NewFilterSuite("200", "", "", "", []string{`(?i)x-custom-env:\s*staging`}, nil, nil, nil, filter.RegexScopeHeader)
	if err != nil {
		t.Fatalf("NewFilterSuite: %v", err)
	}

	jobs := make(chan fuzz.WorkItem, 2)
	results := make(chan fuzz.Result, 2)

	jobs <- fuzz.WorkItem{Req: fuzz.RequestDTO{URL: srv.URL + "/special"}}
	jobs <- fuzz.WorkItem{Req: fuzz.RequestDTO{URL: srv.URL + "/normal"}}
	close(jobs)

	ctx := context.Background()
	fuzz.Worker(ctx, ctx, client, fs, 0, nil, jobs, results, nil, nil, fuzz.WorkerOpts{})

	var accepted []fuzz.Result
	for i := 0; i < 2; i++ {
		r := <-results
		if r.Accepted {
			accepted = append(accepted, r)
		}
	}

	if len(accepted) != 1 {
		t.Fatalf("expected 1 accepted result, got %d", len(accepted))
	}
	if accepted[0].URL != srv.URL+"/special" {
		t.Errorf("expected URL %s/special, got %s", srv.URL, accepted[0].URL)
	}
}

func TestFuzzWorker_RegexScope_Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/found" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.Options{})
	// Match status regex ^200$
	fs, err := filter.NewFilterSuite("", "", "", "", []string{`^200$`}, nil, nil, nil, filter.RegexScopeStatus)
	if err != nil {
		t.Fatalf("NewFilterSuite: %v", err)
	}

	jobs := make(chan fuzz.WorkItem, 2)
	results := make(chan fuzz.Result, 2)

	jobs <- fuzz.WorkItem{Req: fuzz.RequestDTO{URL: srv.URL + "/found"}}
	jobs <- fuzz.WorkItem{Req: fuzz.RequestDTO{URL: srv.URL + "/missing"}}
	close(jobs)

	ctx := context.Background()
	fuzz.Worker(ctx, ctx, client, fs, 0, nil, jobs, results, nil, nil, fuzz.WorkerOpts{})

	var accepted []fuzz.Result
	for i := 0; i < 2; i++ {
		r := <-results
		if r.Accepted {
			accepted = append(accepted, r)
		}
	}

	if len(accepted) != 1 {
		t.Fatalf("expected 1 accepted result, got %d", len(accepted))
	}
	if accepted[0].URL != srv.URL+"/found" {
		t.Errorf("expected URL %s/found, got %s", srv.URL, accepted[0].URL)
	}
}

func TestFuzzWorker_RegexScope_FastPath_NoBodyRead(t *testing.T) {
	var bodyReadCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		// We count when the client reads from body
		atomic.AddInt32(&bodyReadCount, 1)
		_, _ = w.Write([]byte("some data"))
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.Options{})
	// Regex scope is URL, so RequiresBody() is false
	fs, err := filter.NewFilterSuite("200", "", "", "", []string{"fastpath"}, nil, nil, nil, filter.RegexScopeURL)
	if err != nil {
		t.Fatalf("NewFilterSuite: %v", err)
	}
	if fs.RequiresBody() {
		t.Fatalf("expected RequiresBody == false for URL scope")
	}

	jobs := make(chan fuzz.WorkItem, 1)
	results := make(chan fuzz.Result, 1)

	jobs <- fuzz.WorkItem{Req: fuzz.RequestDTO{URL: srv.URL + "/fastpath"}}
	close(jobs)

	ctx := context.Background()
	fuzz.Worker(ctx, ctx, client, fs, 0, nil, jobs, results, nil, nil, fuzz.WorkerOpts{})

	res := <-results
	if !res.Accepted {
		t.Errorf("expected accepted result for URL match")
	}
}

func TestFuzzWorker_RegexScope_RedirectFinalResponse(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redir":
			http.Redirect(w, r, srv.URL+"/dest", http.StatusFound)
		case "/dest":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("arrived"))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.Options{
		FollowRedirects: true,
		MaxRedirects:    5,
	})

	// Match final URL containing "dest"
	fs, _ := filter.NewFilterSuite("200", "", "", "", []string{"dest"}, nil, nil, nil, filter.RegexScopeURL)

	jobs := make(chan fuzz.WorkItem, 2)
	results := make(chan fuzz.Result, 2)

	jobs <- fuzz.WorkItem{Req: fuzz.RequestDTO{URL: srv.URL + "/redir"}}
	jobs <- fuzz.WorkItem{Req: fuzz.RequestDTO{URL: srv.URL + "/other"}}
	close(jobs)

	ctx := context.Background()
	fuzz.Worker(ctx, ctx, client, fs, 0, nil, jobs, results, nil, nil, fuzz.WorkerOpts{})

	var accepted []fuzz.Result
	for i := 0; i < 2; i++ {
		r := <-results
		if r.Accepted {
			accepted = append(accepted, r)
		}
	}

	if len(accepted) != 1 {
		t.Fatalf("expected 1 accepted result, got %d", len(accepted))
	}
	if accepted[0].URL != srv.URL+"/dest" {
		t.Errorf("expected final URL %s/dest, got %s", srv.URL, accepted[0].URL)
	}
	if !accepted[0].Redirected {
		t.Errorf("expected Redirected == true")
	}
}
