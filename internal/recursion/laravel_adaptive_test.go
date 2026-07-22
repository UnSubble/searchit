package recursion_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Add("Set-Cookie", "laravel_session=xyz123abc; Path=/; HttpOnly")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

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
		a.FingerprintCache,
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
	// Wordlist has ".env" and "artisan" as duplicates to test deduplication.
	m := recursion.NewManager(
		a.HTTPClient,
		cfg.Status.Exclude,
		testStaticReader{words: []string{".env", "artisan", ".env", "artisan"}},
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
	// Worker counts: 1, 2, 4, 8, 16, 32, 64, 128
	workerCounts := []int{1, 2, 4, 8, 16, 32, 64, 128}

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

func TestAdaptive_MaxDepthBoundaryCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Add("Set-Cookie", "laravel_session=xyz123abc; Path=/; HttpOnly")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// 1. MaxDepth = 0 boundary check (programmatic edge case)
	{
		cfg := config.Default()
		cfg.Adaptive = true
		cfg.Recursive = true
		cfg.MaxDepth = 0

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

		// With MaxDepth = 0, only the seed URL itself should be scanned.
		if len(results) != 1 {
			t.Errorf("Expected exactly 1 result for MaxDepth=0, got %d: %+v", len(results), results)
		}
	}

	// 2. MaxDepth = 1 boundary check
	{
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

		// Root (0) + wordlist (1) + 5 injected paths (1) = 7 results.
		if len(results) != 7 {
			t.Errorf("Expected exactly 7 results for MaxDepth=1, got %d: %+v", len(results), results)
		}
	}

	// 3. MaxDepth = 2 boundary check
	{
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

		// Root (1) + depth-1 wordlist/injected (6) + depth-2 recursion (6) = 13 results.
		if len(results) != 13 {
			t.Errorf("Expected exactly 13 results for MaxDepth=2, got %d: %+v", len(results), results)
		}
	}
}

func TestAdaptive_BFS_DFS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Add("Set-Cookie", "laravel_session=xyz123abc; Path=/; HttpOnly")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	strategies := []recursion.Strategy{recursion.BFS, recursion.DFS}
	for _, strat := range strategies {
		cfg := config.Default()
		cfg.Adaptive = true
		cfg.Recursive = true
		cfg.MaxDepth = 1

		a := app.New(context.Background(), cfg)
		m := recursion.NewManager(
			a.HTTPClient,
			cfg.Status.Exclude,
			testStaticReader{words: []string{"somepath"}},
			strat,
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

		if len(results) != 7 {
			t.Errorf("Strategy %s: expected 7 results, got %d", strat.String(), len(results))
		}
	}
}

func TestAdaptive_Cancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Add("Set-Cookie", "laravel_session=xyz123abc; Path=/; HttpOnly")
		}
		// Slow down response to allow cancellation to intercept
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := config.Default()
	cfg.Adaptive = true
	cfg.Recursive = true
	cfg.MaxDepth = 2

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := app.New(ctx, cfg)
	m := recursion.NewManager(
		a.HTTPClient,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"path1", "path2", "path3"}},
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

	resChan := m.Run(ctx, []string{srv.URL}, 4)

	// Trigger cancellation immediately after reading first result
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
		cancel() // Cancel context
	}

	// Verify the scan terminated early and didn't hang
	if len(results) == 13 {
		t.Error("Scan did not terminate early despite context cancellation")
	}
}

func TestAdaptive_EmptyWordlist(t *testing.T) {
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
	// Empty wordlist
	m := recursion.NewManager(
		a.HTTPClient,
		cfg.Status.Exclude,
		testStaticReader{words: []string{}},
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

	// Should have root (1) + 5 Laravel paths = 6 results (wordlist is empty, so no depth 2 wordlist additions).
	if len(results) != 6 {
		t.Errorf("Expected 6 results for empty wordlist adaptive scan, got %d: %+v", len(results), results)
	}
}

func TestAdaptive_RobotsSitemapFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" || r.URL.Path == "/sitemap.xml" {
			w.WriteHeader(http.StatusInternalServerError)
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

	// Scan should succeed despite robots/sitemap 500 errors, and still inject Laravel paths (total 7 results).
	if len(results) != 7 {
		t.Errorf("Expected 7 results, got %d: %+v", len(results), results)
	}
}

func TestAdaptive_SchedulerStarvation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		testStaticReader{words: []string{}},
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

	// Run with 128 workers on empty wordlist. Verify it completes instantly and does not hang.
	done := make(chan struct{})
	go func() {
		resChan := m.Run(context.Background(), []string{srv.URL}, 128)
		for range resChan {
		}
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Starvation/Hang detected: scan did not complete within 1 second under 128 workers")
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
				frontier.Push(recursion.NewSliceGenerator([]engine.Job{{
					URL:    childURL,
					Depth:  1,
					Origin: "adaptive",
				}}))
			}
		}
	}
}

func BenchmarkAdaptive_BFS_Overhead(b *testing.B) {
	// Compare BFS overhead with adaptive scanning enabled
	runStrategyBenchmark(b, recursion.BFS, true)
}

func BenchmarkAdaptive_DFS_Overhead(b *testing.B) {
	// Compare DFS overhead with adaptive scanning enabled
	runStrategyBenchmark(b, recursion.DFS, true)
}

func runStrategyBenchmark(b *testing.B, strategy recursion.Strategy, adaptive bool) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Adaptive = adaptive
	cfg.Recursive = true
	cfg.MaxDepth = 1

	a := app.New(context.Background(), cfg)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		m := recursion.NewManager(
			a.HTTPClient,
			cfg.Status.Exclude,
			testStaticReader{words: []string{"w1", "w2", "w3", "w4", "w5"}},
			strategy,
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
