package engine_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"golang.org/x/time/rate"
)

func TestCanonicalizeLocation(t *testing.T) {
	baseURL, _ := url.Parse("http://host:1337/admin/users")

	tests := []struct {
		name        string
		rawLoc      string
		reqURL      *url.URL
		expectedLoc string
	}{
		{
			name:        "Absolute redirect",
			rawLoc:      "http://example.com/admin/",
			reqURL:      baseURL,
			expectedLoc: "http://example.com/admin/",
		},
		{
			name:        "Root-relative redirect",
			rawLoc:      "/api/",
			reqURL:      baseURL,
			expectedLoc: "http://host:1337/api/",
		},
		{
			name:        "Relative redirect",
			rawLoc:      "js/",
			reqURL:      baseURL,
			expectedLoc: "http://host:1337/admin/js/",
		},
		{
			name:        "Parent traversal",
			rawLoc:      "../login",
			reqURL:      baseURL,
			expectedLoc: "http://host:1337/login",
		},
		{
			name:        "Absolute external redirect",
			rawLoc:      "https://www.google.com/",
			reqURL:      baseURL,
			expectedLoc: "https://www.google.com/",
		},
		{
			name:        "Invalid Location header",
			rawLoc:      "::invalid::",
			reqURL:      baseURL,
			expectedLoc: "::invalid::",
		},
		{
			name:        "Empty raw location",
			rawLoc:      "",
			reqURL:      baseURL,
			expectedLoc: "",
		},
		{
			name:        "Nil request URL",
			rawLoc:      "/api/",
			reqURL:      nil,
			expectedLoc: "/api/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := engine.CanonicalizeLocation(tt.rawLoc, tt.reqURL)
			if actual != tt.expectedLoc {
				t.Errorf("CanonicalizeLocation mismatch:\n  got:  %q\n  want: %q", actual, tt.expectedLoc)
			}
		})
	}
}

func TestRedirectLocation_EndToEndWorker(t *testing.T) {
	tests := []struct {
		name           string
		requestPath    string
		locationHeader string
		expectedLoc    string
	}{
		{
			name:           "Absolute redirect",
			requestPath:    "/admin",
			locationHeader: "http://example.com/admin/",
			expectedLoc:    "http://example.com/admin/",
		},
		{
			name:           "Root-relative redirect",
			requestPath:    "/api",
			locationHeader: "/api/",
			expectedLoc:    "/api/",
		},
		{
			name:           "Relative redirect",
			requestPath:    "/js",
			locationHeader: "js/",
			expectedLoc:    "/js/",
		},
		{
			name:           "Parent traversal",
			requestPath:    "/admin/users",
			locationHeader: "../login",
			expectedLoc:    "/login",
		},
		{
			name:           "Absolute external redirect",
			requestPath:    "/out",
			locationHeader: "https://www.google.com/",
			expectedLoc:    "https://www.google.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", tt.locationHeader)
				w.WriteHeader(http.StatusMovedPermanently)
			}))
			defer server.Close()

			fs, err := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
			if err != nil {
				t.Fatalf("failed to create filter suite: %v", err)
			}

			client := server.Client()
			client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}

			jobs := make(chan engine.Job, 1)
			targetURL := server.URL + tt.requestPath
			jobs <- engine.Job{URL: targetURL}
			close(jobs)

			limiter := rate.NewLimiter(rate.Inf, 0)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			results := engine.Start(
				ctx,
				ctx,
				client,
				fs,
				nil,
				nil,
				1,
				0,
				limiter,
				"GET",
				nil,
				nil,
				"",
				jobs,
				nil,
				nil,
				engine.WorkerOptions{},
			)

			var res engine.Result
			for r := range results {
				res = r
			}

			if res.Headers == nil {
				t.Fatalf("expected non-nil response headers, got res: %+v", res)
			}

			actualLoc := res.Headers.Get("Location")
			var expectedFullLoc string
			if tt.name == "Absolute redirect" || tt.name == "Absolute external redirect" {
				expectedFullLoc = tt.expectedLoc
			} else {
				expectedFullLoc = server.URL + tt.expectedLoc
			}

			if actualLoc != expectedFullLoc {
				t.Errorf("Location header mismatch:\n  got:  %q\n  want: %q", actualLoc, expectedFullLoc)
			}
		})
	}
}
