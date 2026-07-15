package recursion_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/recursion"
)

// staticReader streams a fixed set of words, simulating a wordlist without disk I/O.
type testStaticReader struct {
	words []string
}

func (r testStaticReader) Read(ctx context.Context, out chan<- string) error {
	for _, w := range r.words {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- w:
		}
	}
	return nil
}

func TestAdaptive_DisabledByDefault(t *testing.T) {
	// Start a test server that returns laravel_session cookie on root request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Add("Set-Cookie", "laravel_session=xyz123abc; Path=/; HttpOnly")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// config.Default() must have Adaptive = false by default
	cfg := config.Default()
	cfg.Recursive = true
	cfg.MaxDepth = 2

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
		a.FingerprintCache, // Will be nil since cfg.Adaptive is false
	)

	resChan := m.Run(context.Background(), []string{srv.URL}, 4)
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
	}

	// Verify no Laravel paths were injected
	expectedPaths := []string{".env", "artisan", "storage", "bootstrap", "vendor"}
	for _, p := range expectedPaths {
		for _, r := range results {
			if strings.HasSuffix(strings.TrimRight(r.URL, "/"), p) {
				t.Errorf("Expected path %q to NOT be injected when adaptive scanning is disabled by default", p)
			}
		}
	}
}

func TestAdaptive_EnabledExplicitly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Add("Set-Cookie", "laravel_session=xyz123abc; Path=/; HttpOnly")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	cfg.Adaptive = true // Explicitly enabled
	cfg.Recursive = true
	cfg.MaxDepth = 2

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

	expectedPaths := []string{".env", "artisan", "storage", "bootstrap", "vendor"}
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
		t.Errorf("Expected all %d Laravel paths to be injected when explicitly enabled, got %d", len(expectedPaths), injectedCount)
	}
}

func TestAdaptive_LaravelNotDetected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	expectedPaths := []string{".env", "artisan", "storage", "bootstrap", "vendor"}
	for _, p := range expectedPaths {
		for _, r := range results {
			if strings.HasSuffix(strings.TrimRight(r.URL, "/"), p) {
				t.Errorf("Path %q was injected even though Laravel was not detected", p)
			}
		}
	}
}

func TestAdaptive_DeduplicationAndDuplicateDiscoveries(t *testing.T) {
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
		testStaticReader{words: []string{".env", "artisan"}},
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

	counts := make(map[string]int)
	for _, r := range results {
		counts[r.URL]++
	}

	for u, count := range counts {
		if count > 1 {
			t.Errorf("Expected URL %q to be scanned exactly once, but was scanned %d times", u, count)
		}
	}
}

func TestAdaptive_MultiTargetIsolation(t *testing.T) {
	srvLaravel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Add("Set-Cookie", "laravel_session=xyz123abc; Path=/; HttpOnly")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srvLaravel.Close)

	srvPlain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srvPlain.Close)

	cfg := config.Default()
	cfg.Adaptive = true
	cfg.Recursive = true
	cfg.MaxDepth = 2

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

	resChan1 := m.Run(context.Background(), []string{srvLaravel.URL}, 4)
	var results1 []engine.Result
	for r := range resChan1 {
		results1 = append(results1, r)
	}

	resChan2 := m.Run(context.Background(), []string{srvPlain.URL}, 4)
	var results2 []engine.Result
	for r := range resChan2 {
		results2 = append(results2, r)
	}

	foundLaravelT1 := false
	for _, r := range results1 {
		if strings.HasSuffix(strings.TrimRight(r.URL, "/"), ".env") {
			foundLaravelT1 = true
			break
		}
	}

	foundLaravelT2 := false
	for _, r := range results2 {
		if strings.HasSuffix(strings.TrimRight(r.URL, "/"), ".env") {
			foundLaravelT2 = true
			break
		}
	}

	if !foundLaravelT1 {
		t.Error("Expected Target 1 (Laravel) to have Laravel paths injected")
	}
	if foundLaravelT2 {
		t.Error("Expected Target 2 (Plain) to NOT have Laravel paths injected")
	}
}

func TestAdaptive_WorkerCountsDeterminism(t *testing.T) {
	workerCounts := []int{1, 2, 4, 8, 16, 32, 64}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Add("Set-Cookie", "laravel_session=xyz123abc; Path=/; HttpOnly")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	for _, w := range workerCounts {
		t.Run(fmt.Sprintf("Workers-%d", w), func(t *testing.T) {
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

			resChan := m.Run(context.Background(), []string{srv.URL}, w)
			var results []engine.Result
			for r := range resChan {
				results = append(results, r)
			}

			if len(results) != 7 {
				t.Errorf("Determinism failure: expected 7 scanned results under %d workers, got %d. Results: %+v", w, len(results), results)
			}
		})
	}
}

func TestAdaptive_Redirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		if r.URL.Path == "/" {
			w.Header().Add("Set-Cookie", "laravel_session=xyz123abc; Path=/; HttpOnly")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	cfg.Adaptive = true
	cfg.FollowRedirects = true
	cfg.Recursive = true
	cfg.MaxDepth = 2

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

	resChan := m.Run(context.Background(), []string{srv.URL + "/redirect"}, 4)
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
	}

	foundLaravel := false
	for _, r := range results {
		if strings.HasSuffix(strings.TrimRight(r.URL, "/"), ".env") {
			foundLaravel = true
			break
		}
	}

	if !foundLaravel {
		t.Error("Expected Laravel paths to be injected even when accessed via redirect")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkAdaptive_DetectionOverhead(b *testing.B) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("bench.com")
	fp.AddSignal(fingerprint.Signal{Source: "cookie", Value: "laravel_session"})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		matcher := fingerprint.NewMatcher()
		_ = matcher.Match(fp)
	}
}

func BenchmarkAdaptive_SchedulerOverhead(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Add("Set-Cookie", "laravel_session=xyz123abc; Path=/; HttpOnly")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Adaptive = true
	cfg.Recursive = true
	cfg.MaxDepth = 1

	a := app.New(context.Background(), cfg)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		m := recursion.NewManager(
			a.HTTPClient,
			cfg.Status.Exclude,
			testStaticReader{words: []string{"word1", "word2", "word3"}},
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
		ch := m.Run(context.Background(), []string{srv.URL}, 4)
		for range ch {
		}
	}
}

func BenchmarkAdaptive_PathInjectionOverhead(b *testing.B) {
	visited := make(map[string]struct{})
	frontier := recursion.NewFrontier(recursion.BFS)
	laravelPaths := []string{".env", "artisan", "storage/", "bootstrap/", "vendor/"}
	baseURL := "http://example.com"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for _, p := range laravelPaths {
			childURL := baseURL + "/" + p
			if _, seen := visited[childURL]; !seen {
				visited[childURL] = struct{}{}
				frontier.Push(engine.Job{
					URL:    childURL,
					Depth:  1,
					Origin: "adaptive",
				})
			}
		}
	}
}
