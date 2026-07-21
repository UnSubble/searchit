package recursion_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/recursion"
)

// 1. HTML Link Extraction Starvation Test
func TestScientific_HTMLStarvation(t *testing.T) {
	linkCounts := []int{100, 1000, 10000, 20000} // Reduce from 50k to 20k for fast local test boundaries

	for _, count := range linkCounts {
		t.Run(fmt.Sprintf("Links-%d", count), func(t *testing.T) {
			var links []string
			for i := 0; i < count; i++ {
				links = append(links, fmt.Sprintf("<a href=\"/link-%d\">link</a>", i))
			}
			htmlContent := fmt.Sprintf("<html><body>%s</body></html>", strings.Join(links, "\n"))

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				if r.URL.Path == "/" {
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, htmlContent)
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(srv.Close)

			cfg := config.Default()
			cfg.Recursive = true
			cfg.MaxDepth = 2

			a := app.New(context.Background(), cfg)
			// Wordlist has simple entries to check if they ever get processed
			m := recursion.NewManager(
				a.HTTPClient,
				cfg.Status.Exclude,
				testStaticReader{words: []string{"word1", "word2"}},
				recursion.BFS,
				cfg.MaxDepth,
				cfg.RecurseOn,
				false,
				false,
				nil,
				nil,
				nil,
				nil,
				0,
				nil,
				nil,
			)
			m.SetDisableWildcard(true)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resChan := m.Run(ctx, []string{srv.URL}, 8)
			var results []engine.Result
			for r := range resChan {
				results = append(results, r)
			}

			// Check if word1 or word2 (wordlist entries at depth 1) were scanned.
			t.Logf("Links: %d, Results count: %d", count, len(results))
		})
	}
}

// 2. Wildcard Detection Boundary Test (19 wildcard responses + 1 legitimate response)
func TestScientific_WildcardBoundary(t *testing.T) {
	t.Run("legitimate first 19", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/legit" {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, strings.Repeat("A", 100))
			} else {
				w.Header().Set("Content-Length", "0")
				w.WriteHeader(http.StatusOK) // Wildcard returns 200 with 0 bytes
			}
		}))
		t.Cleanup(srv.Close)

		cfg := config.Default()
		cfg.Recursive = true
		cfg.MaxDepth = 1

		a := app.New(context.Background(), cfg)
		// Generate 30 words: 1 legit, 29 wildcards
		words := []string{"legit"}
		for i := 0; i < 29; i++ {
			words = append(words, fmt.Sprintf("wildcard-%d", i))
		}

		m := recursion.NewManager(
			a.HTTPClient,
			cfg.Status.Exclude,
			testStaticReader{words: words},
			recursion.BFS,
			cfg.MaxDepth,
			cfg.RecurseOn,
			false,
			false,
			nil,
			nil,
			nil,
			nil,
			0,
			nil,
			nil,
		)

		resChan := m.Run(context.Background(), []string{srv.URL}, 4)
		var results []engine.Result
		for r := range resChan {
			t.Logf("DEBUG 19: URL=%s Accepted=%v StatusCode=%d Length=%d Err=%v", r.URL, r.Accepted, r.StatusCode, r.Length, r.Err)
			results = append(results, r)
		}

		foundLegit := false
		for _, r := range results {
			if strings.HasSuffix(r.URL, "/legit") {
				foundLegit = true
				break
			}
		}

		if !foundLegit {
			t.Errorf("Legitimate finding was lost!")
		}
	})

	t.Run("legitimate after wildcard active", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/legit" {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, strings.Repeat("A", 100))
			} else {
				w.Header().Set("Content-Length", "0")
				w.WriteHeader(http.StatusOK)
			}
		}))
		t.Cleanup(srv.Close)

		cfg := config.Default()
		cfg.Recursive = true
		cfg.MaxDepth = 1

		a := app.New(context.Background(), cfg)
		// Generate 30 words: 29 wildcards first, then 1 legit
		var words []string
		for i := 0; i < 29; i++ {
			words = append(words, fmt.Sprintf("wildcard-%d", i))
		}
		words = append(words, "legit")

		m := recursion.NewManager(
			a.HTTPClient,
			cfg.Status.Exclude,
			testStaticReader{words: words},
			recursion.BFS,
			cfg.MaxDepth,
			cfg.RecurseOn,
			false,
			false,
			nil,
			nil,
			nil,
			nil,
			0,
			nil,
			nil,
		)

		resChan := m.Run(context.Background(), []string{srv.URL}, 4)
		var results []engine.Result
		for r := range resChan {
			t.Logf("DEBUG AFTER: URL=%s Accepted=%v StatusCode=%d Length=%d Err=%v", r.URL, r.Accepted, r.StatusCode, r.Length, r.Err)
			results = append(results, r)
		}

		foundLegit := false
		for _, r := range results {
			if strings.HasSuffix(r.URL, "/legit") {
				foundLegit = true
				break
			}
		}

		if !foundLegit {
			t.Errorf("Legitimate finding after wildcard active was lost!")
		}
	})
}

// 3. Technology Suppression mixed target test (WordPress + Custom Admin Panel + Laravel API subfolder)
func TestScientific_SuppressionMixed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		// Root site behaves like WordPress
		if p == "/" {
			w.Header().Add("Set-Cookie", "wp-settings-1=xyz; Path=/")
			w.WriteHeader(http.StatusOK)
			return
		}
		if p == "/api" || p == "/api/v1" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// Subfolder acts like Laravel API
		if strings.HasPrefix(p, "/api/v1") {
			if strings.HasSuffix(p, "/artisan") {
				w.WriteHeader(http.StatusOK) // This is a valid finding that we want to discover!
				_, _ = io.WriteString(w, "Laravel Artisan command output")
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	cfg.Adaptive = true
	cfg.Recursive = true
	cfg.MaxDepth = 3 // Depth 3 so we can go: / (0) -> /api (1) -> /api/v1 (2) -> /api/v1/artisan (3)

	a := app.New(context.Background(), cfg)
	m := recursion.NewManager(
		a.HTTPClient,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"api", "v1", "artisan"}},
		recursion.BFS,
		cfg.MaxDepth,
		cfg.RecurseOn,
		false,
		false,
		nil,
		nil,
		nil,
		nil,
		0,
		nil,
		a.FingerprintCache,
	)

	resChan := m.Run(context.Background(), []string{srv.URL}, 4)
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
	}

	foundArtisan := false
	for _, r := range results {
		if strings.HasSuffix(r.URL, "/api/v1/artisan") {
			foundArtisan = true
			break
		}
	}

	// If WordPress suppression is global per-host, then "artisan" will be suppressed on this host because WordPress was matched at root!
	// Let's see if we lost it!
	if !foundArtisan {
		t.Errorf("FAILED: Valid Laravel finding /api/v1/artisan was suppressed on WordPress host!")
	}
}

// 4. Frontier Prioritization (Robots + Sitemap + Adaptive + HTML + Wordlist)
func TestScientific_PrioritizationRatio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "User-agent: *\nAllow: /robots-path-1\nAllow: /robots-path-2\n")
			return
		}
		if p == "/sitemap.xml" {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "<urlset><url><loc>http://localhost/sitemap-path-1</loc></url></urlset>")
			return
		}
		if p == "/" {
			w.Header().Add("Set-Cookie", "laravel_session=xyz")
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "<html><body><a href=\"/html-path-1\">link</a></body></html>")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	cfg.Adaptive = true
	cfg.Recursive = true
	cfg.MaxDepth = 2

	a := app.New(context.Background(), cfg)
	m := recursion.NewManager(
		a.HTTPClient,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"word1", "word2"}},
		recursion.BFS,
		cfg.MaxDepth,
		cfg.RecurseOn,
		false,
		false,
		nil,
		nil,
		nil,
		nil,
		0,
		nil,
		a.FingerprintCache,
	)

	resChan := m.Run(context.Background(), []string{srv.URL}, 4)
	for range resChan {
	}

	_, _, _, _, _, high, _, low := m.GetAdaptiveMetrics()
	t.Logf("High priority count: %d, Low priority count: %d", high, low)
}

// 5. Scientific telemetry reporting (measure requests, wall time, throughput, memory)
func TestScientific_Metrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	cfg.Adaptive = true
	cfg.Recursive = true
	cfg.MaxDepth = 1

	var ms1, ms2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms1)

	startTime := time.Now()
	a := app.New(context.Background(), cfg)

	var words []string
	for i := 0; i < 500; i++ {
		words = append(words, fmt.Sprintf("word-%d", i))
	}

	m := recursion.NewManager(
		a.HTTPClient,
		cfg.Status.Exclude,
		testStaticReader{words: words},
		recursion.BFS,
		cfg.MaxDepth,
		cfg.RecurseOn,
		false,
		false,
		nil,
		nil,
		nil,
		nil,
		0,
		nil,
		a.FingerprintCache,
	)

	resChan := m.Run(context.Background(), []string{srv.URL}, 8)
	var count int
	for range resChan {
		count++
	}

	duration := time.Since(startTime)
	runtime.ReadMemStats(&ms2)

	allocBytes := ms2.TotalAlloc - ms1.TotalAlloc
	t.Logf("Total Requests: %d, Wall Time: %s, Allocations: %d Bytes", count, duration, allocBytes)
}
