package recursion_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/recursion"
)

func TestAdaptive_WordPressDetectionAndPathInjection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Add("Set-Cookie", "wp-settings-1=xyz123; Path=/; HttpOnly")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	cfg.Adaptive = true
	cfg.Recursive = true
	cfg.MaxDepth = 1

	a := app.New(context.Background(), cfg)
	m := recursion.NewManager(
		a.HTTPClient,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"somepath"}},
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

	expectedPaths := []string{"wp-admin", "wp-content", "wp-includes", "wp-login.php", "xmlrpc.php"}
	injectedCount := 0
	for _, p := range expectedPaths {
		found := false
		for _, r := range results {
			if strings.HasSuffix(strings.TrimRight(r.URL, "/"), p) {
				found = true
				break
			}
		}
		if found {
			injectedCount++
		}
	}

	if injectedCount != len(expectedPaths) {
		t.Errorf("Expected all %d WordPress paths to be injected, got %d", len(expectedPaths), injectedCount)
	}
}

func TestAdaptive_ExpressDetectionAndPathInjection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Add("X-Powered-By", "Express")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	cfg.Adaptive = true
	cfg.Recursive = true
	cfg.MaxDepth = 1

	a := app.New(context.Background(), cfg)
	m := recursion.NewManager(
		a.HTTPClient,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"somepath"}},
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

	expectedPaths := []string{"api", "uploads", "assets", "static"}
	injectedCount := 0
	for _, p := range expectedPaths {
		found := false
		for _, r := range results {
			if strings.HasSuffix(strings.TrimRight(r.URL, "/"), p) {
				found = true
				break
			}
		}
		if found {
			injectedCount++
		}
	}

	if injectedCount != len(expectedPaths) {
		t.Errorf("Expected all %d Express paths to be injected, got %d", len(expectedPaths), injectedCount)
	}
}

func TestAdaptive_CrossTechnologyNonSuppression(t *testing.T) {
	// 1. WordPress host: should NOT suppress Laravel-specific words (artisan, horizon, telescope)
	t.Run("WordPress does not suppress Laravel words", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				w.Header().Add("Set-Cookie", "wp-settings-1=xyz123; Path=/; HttpOnly")
			}
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		cfg := config.Default()
		cfg.Adaptive = true
		cfg.Recursive = true
		cfg.MaxDepth = 2 // Depth 2 so we recurse on "/" and check candidate words at depth 1

		a := app.New(context.Background(), cfg)
		// We supply wordlist containing some generic words, wordpress words, and laravel words.
		m := recursion.NewManager(
			a.HTTPClient,
			cfg.Status.Exclude,
			testStaticReader{words: []string{"wp-admin", "artisan", "horizon", "hello"}},
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

		// Ensure "hello", "wp-admin" (WordPress), and "artisan", "horizon" (Laravel) are all scanned.
		var foundHello, foundWPAdmin, foundArtisan, foundHorizon bool
		for _, r := range results {
			urlStr := strings.TrimRight(r.URL, "/")
			if strings.HasSuffix(urlStr, "hello") {
				foundHello = true
			}
			if strings.HasSuffix(urlStr, "wp-admin") {
				foundWPAdmin = true
			}
			if strings.HasSuffix(urlStr, "artisan") {
				foundArtisan = true
			}
			if strings.HasSuffix(urlStr, "horizon") {
				foundHorizon = true
			}
		}

		if !foundHello || !foundWPAdmin || !foundArtisan || !foundHorizon {
			t.Errorf("Expected all words to be scanned, hello=%v, wp-admin=%v, artisan=%v, horizon=%v", foundHello, foundWPAdmin, foundArtisan, foundHorizon)
		}
	})

	// 2. Laravel host: should not suppress WordPress-specific words (wp-admin, wp-content, wp-includes)
	t.Run("Laravel does not suppress WordPress words", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				w.Header().Add("Set-Cookie", "laravel_session=xyz123abc; Path=/; HttpOnly")
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
			testStaticReader{words: []string{"artisan", "wp-admin", "wp-content", "hello"}},
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

		// Ensure "hello", "artisan" (Laravel), and "wp-admin", "wp-content" (WordPress) are all scanned.
		var foundHello, foundArtisan, foundWPAdmin, foundWPContent bool
		for _, r := range results {
			urlStr := strings.TrimRight(r.URL, "/")
			if strings.HasSuffix(urlStr, "hello") {
				foundHello = true
			}
			if strings.HasSuffix(urlStr, "artisan") {
				foundArtisan = true
			}
			if strings.HasSuffix(urlStr, "wp-admin") {
				foundWPAdmin = true
			}
			if strings.HasSuffix(urlStr, "wp-content") {
				foundWPContent = true
			}
		}

		if !foundHello || !foundArtisan || !foundWPAdmin || !foundWPContent {
			t.Errorf("Expected all words to be scanned, hello=%v, artisan=%v, wp-admin=%v, wp-content=%v", foundHello, foundArtisan, foundWPAdmin, foundWPContent)
		}
	})
}
