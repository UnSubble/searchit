package recursion_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/status"
)

func TestRegression_RedirectRecursionBug(t *testing.T) {
	// Historical Bug: Searchit did not follow redirect destinations for recursion.
	// It recursed under the original requested URL rather than the redirect location.
	// E.g., /admin -> 301 -> /admin/
	// Correct behavior: children should be /admin/login, not /adminlogin.

	var requestedPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.Path)
		if r.URL.Path == "/admin" {
			w.Header().Set("Location", "/admin/")
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	reader := staticReader{words: []string{"login"}}
	excludeFilters, _ := status.Parse("404")
	recurseOnFilters, _ := status.Parse("200,301")

	manager := recursion.NewManager(
		srv.Client(),
		excludeFilters,
		reader,
		recursion.BFS,
		2, // Depth 2
		recurseOnFilters,
		true,
		true,
		nil, nil, nil, nil, 0, nil, nil,
	)

	resultsChan := manager.Run(context.Background(), []string{srv.URL + "/admin"}, 4)
	for range resultsChan {
	}

	// Verify that the child request was `/admin/login`, not `/adminlogin`
	foundCorrectChild := false
	for _, p := range requestedPaths {
		if p == "/admin/login" {
			foundCorrectChild = true
			break
		}
	}

	if !foundCorrectChild {
		t.Errorf("Redirect recursion bug regression: expected child request path '/admin/login' in requested paths: %v", requestedPaths)
	}
}

func TestRegression_DuplicateSuppression(t *testing.T) {
	// Verify that duplicates inside wordlists and multiple seeds are suppressed correctly
	// and do not trigger duplicate requests to the HTTP server.

	var requestCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// Wordlist with duplicates
	reader := staticReader{words: []string{"test", "test", "test", "other", "other"}}
	excludeFilters, _ := status.Parse("404")

	manager := recursion.NewManager(
		srv.Client(),
		excludeFilters,
		reader,
		recursion.BFS,
		1,
		status.MustParse("200"),
		true,
		true,
		nil, nil, nil, nil, 0, nil, nil,
	)

	// Run with duplicate seed URLs as well
	seeds := []string{srv.URL, srv.URL}
	resultsChan := manager.Run(context.Background(), seeds, 8)
	for range resultsChan {
	}

	// Total expected unique requests:
	// - Root URL (1 unique request)
	// - /robots.txt auto-discovery (1 request)
	// - /sitemap.xml auto-discovery (1 request)
	// - /test (1 unique request)
	// - /other (1 unique request)
	// Total: 5 requests
	expected := int64(5)
	if requestCount != expected {
		t.Errorf("Duplicate suppression bug regression: expected %d requests, got %d", expected, requestCount)
	}
}
