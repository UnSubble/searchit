package engine_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
)

func TestWorker_ExtractLinksCapability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body><a href="/hidden_path">Link</a></body></html>`))
	}))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	a := app.New(context.Background(), cfg)
	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)

	t.Run("ExtractLinksDisabled_LeavesResultLinksNil", func(t *testing.T) {
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL}
		close(jobs)

		engine.Worker(
			context.Background(),
			context.Background(),
			a.HTTPClient,
			fs,
			nil, nil,
			0,
			nil,
			"GET",
			nil,
			nil,
			"",
			jobs,
			results,
			nil,
			nil,
			engine.WorkerOptions{ExtractLinks: false},
		)

		res := <-results
		if len(res.Links) != 0 {
			t.Errorf("expected Result.Links to be empty when ExtractLinks=false, got %v", res.Links)
		}
	})

	t.Run("ExtractLinksEnabled_PopulatesResultLinks", func(t *testing.T) {
		jobs := make(chan engine.Job, 1)
		results := make(chan engine.Result, 1)
		jobs <- engine.Job{URL: srv.URL}
		close(jobs)

		engine.Worker(
			context.Background(),
			context.Background(),
			a.HTTPClient,
			fs,
			nil, nil,
			0,
			nil,
			"GET",
			nil,
			nil,
			"",
			jobs,
			results,
			nil,
			nil,
			engine.WorkerOptions{ExtractLinks: true},
		)

		res := <-results
		if len(res.Links) == 0 {
			t.Fatalf("expected Result.Links to be populated when ExtractLinks=true, got empty")
		}
		found := false
		for _, l := range res.Links {
			if l == "/hidden_path" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected link '/hidden_path' in Result.Links, got %v", res.Links)
		}
	})
}
