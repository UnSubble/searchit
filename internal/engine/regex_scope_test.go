package engine_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/httpclient"
	"github.com/unsubble/searchit/internal/stats"
)

func TestEngineWorker_RegexScope_URL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response body does not have the magic word"))
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.Options{})
	// Match URL containing "admin"
	fs, err := filter.NewFilterSuite("200", "", "", "", []string{"admin"}, nil, nil, nil, filter.RegexScopeURL)
	if err != nil {
		t.Fatalf("NewFilterSuite: %v", err)
	}

	jobs := make(chan engine.Job, 2)
	results := make(chan engine.Result, 2)

	jobs <- engine.Job{URL: srv.URL + "/admin"}
	jobs <- engine.Job{URL: srv.URL + "/user"}
	close(jobs)

	collector := stats.NewCollector()
	ctx := context.Background()
	engine.Worker(ctx, ctx, client, fs, nil, nil, 0, nil, "GET", nil, nil, "", jobs, results, collector, nil, engine.WorkerOptions{})

	var accepted []engine.Result
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
		t.Errorf("expected URL to be %s/admin, got %s", srv.URL, accepted[0].URL)
	}
}

func TestEngineWorker_RegexScope_Header(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/secret" {
			w.Header().Set("X-Auth-Token", "supersecret123")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.Options{})
	// Match header containing "X-Auth-Token"
	fs, err := filter.NewFilterSuite("200", "", "", "", []string{`(?i)x-auth-token:\s*supersecret`}, nil, nil, nil, filter.RegexScopeHeader)
	if err != nil {
		t.Fatalf("NewFilterSuite: %v", err)
	}

	jobs := make(chan engine.Job, 2)
	results := make(chan engine.Result, 2)

	jobs <- engine.Job{URL: srv.URL + "/secret"}
	jobs <- engine.Job{URL: srv.URL + "/public"}
	close(jobs)

	collector := stats.NewCollector()
	ctx := context.Background()
	engine.Worker(ctx, ctx, client, fs, nil, nil, 0, nil, "GET", nil, nil, "", jobs, results, collector, nil, engine.WorkerOptions{})

	var accepted []engine.Result
	for i := 0; i < 2; i++ {
		r := <-results
		if r.Accepted {
			accepted = append(accepted, r)
		}
	}

	if len(accepted) != 1 {
		t.Fatalf("expected 1 accepted result, got %d", len(accepted))
	}
	if accepted[0].URL != srv.URL+"/secret" {
		t.Errorf("expected /secret, got %s", accepted[0].URL)
	}
}

func TestEngineWorker_RegexScope_RedirectFinalResponse(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			http.Redirect(w, r, srv.URL+"/dashboard", http.StatusFound)
		case "/dashboard":
			w.Header().Set("X-Landing", "final-page")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("welcome to dashboard"))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("other"))
		}
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.Options{
		FollowRedirects: true,
		MaxRedirects:    5,
	})

	// Match regex against final URL (should match "dashboard", not "login")
	fs, err := filter.NewFilterSuite("200", "", "", "", []string{`dashboard`}, nil, nil, nil, filter.RegexScopeURL)
	if err != nil {
		t.Fatalf("NewFilterSuite: %v", err)
	}

	jobs := make(chan engine.Job, 2)
	results := make(chan engine.Result, 2)

	jobs <- engine.Job{URL: srv.URL + "/login"}
	jobs <- engine.Job{URL: srv.URL + "/other"}
	close(jobs)

	ctx := context.Background()
	engine.Worker(ctx, ctx, client, fs, nil, nil, 0, nil, "GET", nil, nil, "", jobs, results, nil, nil, engine.WorkerOptions{})

	var accepted []engine.Result
	for i := 0; i < 2; i++ {
		r := <-results
		if r.Accepted {
			accepted = append(accepted, r)
		}
	}

	if len(accepted) != 1 {
		t.Fatalf("expected 1 accepted result for redirected destination, got %d", len(accepted))
	}
	if accepted[0].URL != srv.URL+"/dashboard" {
		t.Errorf("expected final URL %s/dashboard, got %s", srv.URL, accepted[0].URL)
	}
	if !accepted[0].Redirected {
		t.Errorf("expected Redirected == true")
	}
}

func TestEngineWorker_RegexScope_Accounting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test"))
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.Options{})
	// Match only /keep
	fs, _ := filter.NewFilterSuite("200", "", "", "", []string{"keep"}, nil, nil, nil, filter.RegexScopeURL)

	jobs := make(chan engine.Job, 3)
	results := make(chan engine.Result, 3)

	jobs <- engine.Job{URL: srv.URL + "/keep"}
	jobs <- engine.Job{URL: srv.URL + "/drop1"}
	jobs <- engine.Job{URL: srv.URL + "/drop2"}
	close(jobs)

	collector := stats.NewCollector()
	collector.RecordJobProduced()
	collector.RecordJobProduced()
	collector.RecordJobProduced()

	ctx := context.Background()
	engine.Worker(ctx, ctx, client, fs, nil, nil, 0, nil, "GET", nil, nil, "", jobs, results, collector, nil, engine.WorkerOptions{})

	var accepted, filtered int
	for i := 0; i < 3; i++ {
		r := <-results
		if r.Accepted {
			accepted++
		} else {
			filtered++
		}
	}

	if accepted != 1 || filtered != 2 {
		t.Errorf("expected 1 accepted and 2 filtered, got %d accepted, %d filtered", accepted, filtered)
	}

	// Verify collector stats
	snapshot := collector.Snapshot()
	if snapshot.ResponsesReceived != 3 {
		t.Errorf("expected 3 responses received, got %d", snapshot.ResponsesReceived)
	}
	if snapshot.RequestsFiltered != 2 {
		t.Errorf("expected 2 filtered in collector, got %d", snapshot.RequestsFiltered)
	}
}
