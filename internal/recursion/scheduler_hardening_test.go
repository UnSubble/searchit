package recursion_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/recursion"
)

type mockRoundTripper struct {
	mu       sync.Mutex
	reqs     []string
	response func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	m.reqs = append(m.reqs, req.URL.String())
	m.mu.Unlock()
	return m.response(req)
}

type errorReader struct {
	err error
}

func (r errorReader) Read(ctx context.Context, out chan<- string) error {
	return r.err
}

func TestHarden_ZeroTargets(t *testing.T) {
	cfg := config.Default()
	cfg.Recursive = true

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	m := recursion.NewManager(
		client,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"some"}},
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

	done := make(chan struct{})
	go func() {
		resChan := m.Run(context.Background(), []string{}, 4)
		for range resChan {
		}
		close(done)
	}()

	select {
	case <-done:
		// Success: exited cleanly with zero targets
	case <-time.After(100 * time.Millisecond):
		t.Error("Zero targets scan hung")
	}
}

func TestHarden_OneTarget(t *testing.T) {
	cfg := config.Default()
	cfg.Recursive = true
	cfg.MaxDepth = 1

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	m := recursion.NewManager(
		client,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"some"}},
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

	resChan := m.Run(context.Background(), []string{"http://target1.com"}, 4)
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
	}

	// 1 seed + 1 wordlist path = 2 results
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d: %+v", len(results), results)
	}
}

func TestHarden_OneHundredTargets(t *testing.T) {
	cfg := config.Default()
	cfg.Recursive = true
	cfg.MaxDepth = 1

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	m := recursion.NewManager(
		client,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"some"}},
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

	var targets []string
	for i := 0; i < 100; i++ {
		targets = append(targets, fmt.Sprintf("http://target-%d.com", i))
	}

	resChan := m.Run(context.Background(), targets, 16)
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
	}

	// 100 seeds + 100 wordlist paths = 200 results
	if len(results) != 200 {
		t.Errorf("Expected 200 results, got %d", len(results))
	}
}

func TestHarden_EmptyWordlists(t *testing.T) {
	cfg := config.Default()
	cfg.Recursive = true
	cfg.MaxDepth = 2

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	m := recursion.NewManager(
		client,
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
		nil,
	)

	resChan := m.Run(context.Background(), []string{"http://emptywordlist.com"}, 4)
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
	}

	// Only 1 seed scanned
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

func TestHarden_DuplicatedWordlists(t *testing.T) {
	cfg := config.Default()
	cfg.Recursive = true
	cfg.MaxDepth = 2

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	m := recursion.NewManager(
		client,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"dup", "dup", "another", "another"}},
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

	resChan := m.Run(context.Background(), []string{"http://duplicatedwordlist.com"}, 4)
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
	}

	// Root (1) + unique words (2) + unique words at depth 2 (2*2 = 4) = 7 results.
	if len(results) != 7 {
		t.Errorf("Expected 7 results, got %d: %+v", len(results), results)
	}
}

func TestHarden_MaxDepthBoundary(t *testing.T) {
	depths := []uint16{0, 1, 2, 5, 10}

	for _, d := range depths {
		t.Run(fmt.Sprintf("Depth-%d", d), func(t *testing.T) {
			cfg := config.Default()
			cfg.Recursive = true
			cfg.MaxDepth = d

			rt := &mockRoundTripper{
				response: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader(nil)),
					}, nil
				},
			}
			client := &http.Client{Transport: rt}

			m := recursion.NewManager(
				client,
				cfg.Status.Exclude,
				testStaticReader{words: []string{"some"}},
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

			resChan := m.Run(context.Background(), []string{"http://depthtarget.com"}, 4)
			var results []engine.Result
			for r := range resChan {
				results = append(results, r)
			}

			// Expected count: For each depth level, we have exactly 1 path.
			// At Depth M, we scan exactly 1 path (which is seed/some/some/.../some).
			// So total results = d + 1 (except for d = 0, which yields 1).
			expected := int(d) + 1
			if d == 0 {
				expected = 1
			}
			if len(results) != expected {
				t.Errorf("Expected %d results for maxDepth=%d, got %d: %+v", expected, d, len(results), results)
			}
		})
	}
}

func TestHarden_WorkerCounts(t *testing.T) {
	workerCounts := []int{1, 2, 4, 8, 16, 32, 64, 128}

	for _, w := range workerCounts {
		t.Run(fmt.Sprintf("Workers-%d", w), func(t *testing.T) {
			cfg := config.Default()
			cfg.Recursive = true
			cfg.MaxDepth = 3

			rt := &mockRoundTripper{
				response: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader(nil)),
					}, nil
				},
			}
			client := &http.Client{Transport: rt}

			m := recursion.NewManager(
				client,
				cfg.Status.Exclude,
				testStaticReader{words: []string{"w"}},
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

			resChan := m.Run(context.Background(), []string{"http://workertarget.com"}, w)
			var results []engine.Result
			for r := range resChan {
				results = append(results, r)
			}

			// Expected results: Depth 0 (1), Depth 1 (1), Depth 2 (1), Depth 3 (1) = 4 results
			if len(results) != 4 {
				t.Errorf("Determinism failure under %d workers: expected 4 results, got %d: %+v", w, len(results), results)
			}
		})
	}
}

func TestHarden_BFS_DFS(t *testing.T) {
	cfg := config.Default()
	cfg.Recursive = true
	cfg.MaxDepth = 2

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	// BFS traversal
	mBFS := recursion.NewManager(
		client,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"a", "b"}},
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

	resChanBFS := mBFS.Run(context.Background(), []string{"http://bfsdfs.com"}, 1)
	var resultsBFS []string
	for r := range resChanBFS {
		resultsBFS = append(resultsBFS, r.URL)
	}

	// DFS traversal
	mDFS := recursion.NewManager(
		client,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"a", "b"}},
		recursion.DFS,
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

	resChanDFS := mDFS.Run(context.Background(), []string{"http://bfsdfs.com"}, 1)
	var resultsDFS []string
	for r := range resChanDFS {
		resultsDFS = append(resultsDFS, r.URL)
	}

	// Verify both got same total paths scanned
	if len(resultsBFS) != len(resultsDFS) {
		t.Errorf("Total results mismatch: BFS=%d, DFS=%d", len(resultsBFS), len(resultsDFS))
	}

	// Assert BFS order vs DFS order (DFS traverses deep first, BFS traverses level-by-level).
	// BFS: http://bfsdfs.com, http://bfsdfs.com/a, http://bfsdfs.com/b, http://bfsdfs.com/a/a, http://bfsdfs.com/a/b, ...
	// DFS: http://bfsdfs.com, http://bfsdfs.com/b, http://bfsdfs.com/b/b, http://bfsdfs.com/b/a, http://bfsdfs.com/a, ...
	if resultsBFS[1] == resultsDFS[1] && resultsBFS[2] == resultsDFS[2] && resultsBFS[3] == resultsDFS[3] {
		// BFS and DFS must not produce identical order of traversed URLs
		// (unless they are trivial size 1/2)
		t.Errorf("BFS and DFS ordered URLs identically: %+v", resultsBFS)
	}
}

func TestHarden_AdaptiveToggle(t *testing.T) {
	cfg := config.Default()
	cfg.Adaptive = true
	cfg.Recursive = true
	cfg.MaxDepth = 1

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			header := http.Header{}
			header.Add("Set-Cookie", "laravel_session=xyz123abc; Path=/; HttpOnly")
			return &http.Response{
				StatusCode: 200,
				Header:     header,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}

	// 1. Enabled
	a := app.New(context.Background(), cfg)
	a.HTTPClient.Transport = app.WrapTransport(rt, a.FingerprintCache)
	mEnabled := recursion.NewManager(
		a.HTTPClient,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"some"}},
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

	resChan1 := mEnabled.Run(context.Background(), []string{"http://adaptivetoggle.com"}, 4)
	var results1 []engine.Result
	for r := range resChan1 {
		results1 = append(results1, r)
	}

	// Root (1) + wordlist (1) + 5 injected = 7 results
	if len(results1) != 7 {
		t.Errorf("Expected 7 results when adaptive is enabled, got %d: %+v", len(results1), results1)
	}

	// 2. Disabled
	cfg.Adaptive = false
	a2 := app.New(context.Background(), cfg)
	a2.HTTPClient.Transport = rt
	mDisabled := recursion.NewManager(
		a2.HTTPClient,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"some"}},
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

	resChan2 := mDisabled.Run(context.Background(), []string{"http://adaptivetoggle.com"}, 4)
	var results2 []engine.Result
	for r := range resChan2 {
		results2 = append(results2, r)
	}

	// Root (1) + wordlist (1) = 2 results
	if len(results2) != 2 {
		t.Errorf("Expected 2 results when adaptive is disabled, got %d", len(results2))
	}
}

func TestHarden_Redirects(t *testing.T) {
	cfg := config.Default()
	cfg.Adaptive = true
	cfg.FollowRedirects = true
	cfg.Recursive = true
	cfg.MaxDepth = 1

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/redirect" {
				header := http.Header{}
				header.Add("Location", "/laravel")
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     header,
					Body:       io.NopCloser(bytes.NewReader(nil)),
				}, nil
			}
			if req.URL.Path == "/laravel" {
				header := http.Header{}
				header.Add("Set-Cookie", "laravel_session=xyz123; Path=/; HttpOnly")
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     header,
					Body:       io.NopCloser(bytes.NewReader(nil)),
				}, nil
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}

	a := app.New(context.Background(), cfg)
	// Override transport
	a.HTTPClient.Transport = app.WrapTransport(rt, a.FingerprintCache)

	m := recursion.NewManager(
		a.HTTPClient,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"some"}},
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

	// Inject custom roundtripper directly inside manager client
	resChan := m.Run(context.Background(), []string{"http://redirecttarget.com/redirect"}, 1)
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
	}

	// Verify Laravel paths are successfully injected after following redirects
	foundLaravel := false
	for _, r := range results {
		if strings.HasSuffix(strings.TrimRight(r.URL, "/"), ".env") {
			foundLaravel = true
			break
		}
	}
	if !foundLaravel {
		t.Error("Expected Laravel paths to be injected after following redirect, but not found")
	}
}

func TestHarden_Cancellation(t *testing.T) {
	cfg := config.Default()
	cfg.Recursive = true
	cfg.MaxDepth = 3

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			time.Sleep(10 * time.Millisecond)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	m := recursion.NewManager(
		client,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"a", "b", "c"}},
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resChan := m.Run(ctx, []string{"http://cancel.com"}, 4)
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
		cancel() // Cancel scan immediately
	}

	if len(results) >= 40 {
		t.Error("Scan did not cancel cleanly")
	}
}

func TestHarden_RobotsSitemapFailures(t *testing.T) {
	cfg := config.Default()
	cfg.Recursive = true
	cfg.MaxDepth = 1

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "robots.txt") || strings.Contains(req.URL.Path, "sitemap") {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewReader(nil)),
				}, nil
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	m := recursion.NewManager(
		client,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"some"}},
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

	resChan := m.Run(context.Background(), []string{"http://failures.com"}, 4)
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
	}

	// 1 seed + 1 wordlist path = 2 results
	if len(results) != 2 {
		t.Errorf("Expected 2 results despite robots/sitemap failures, got %d", len(results))
	}
}

func TestHarden_RecursionFailures(t *testing.T) {
	cfg := config.Default()
	cfg.Recursive = true
	cfg.MaxDepth = 2

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	// Reader returns error. Verification: scan finishes cleanly, already enqueued jobs are not cancelled.
	errReader := errorReader{err: fmt.Errorf("read error")}

	m := recursion.NewManager(
		client,
		cfg.Status.Exclude,
		errReader,
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

	resChan := m.Run(context.Background(), []string{"http://recfailures.com"}, 4)
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
	}

	// Root only scanned (wordlist reader failed)
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

func TestHarden_MalformedURLs(t *testing.T) {
	cfg := config.Default()
	cfg.Recursive = true
	cfg.MaxDepth = 1

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	m := recursion.NewManager(
		client,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"some"}},
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

	// Scan with a malformed URL alongside normal URL
	resChan := m.Run(context.Background(), []string{"http://normal.com", "::malformed::"}, 4)
	var results []string
	for r := range resChan {
		results = append(results, r.URL)
	}

	// Normal seed + normal seed child = 2 results (malformed URL skipped cleanly)
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d: %+v", len(results), results)
	}
}

func TestHarden_DuplicateDiscoveries(t *testing.T) {
	cfg := config.Default()
	cfg.Recursive = true
	cfg.MaxDepth = 2

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	m := recursion.NewManager(
		client,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"path1", "path1", "path2"}},
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

	// Double seed url list. Verify deduplication.
	resChan := m.Run(context.Background(), []string{"http://dups.com", "http://dups.com"}, 4)
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
	}

	// Unique URL checks
	urls := make(map[string]int)
	for _, r := range results {
		urls[r.URL]++
	}

	for u, count := range urls {
		if count > 1 {
			t.Errorf("URL %s was scanned multiple times: %d", u, count)
		}
	}
}

func TestHarden_RecursivePathInjections(t *testing.T) {
	cfg := config.Default()
	cfg.Adaptive = true
	cfg.Recursive = true
	cfg.MaxDepth = 2

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			header := http.Header{}
			if req.URL.Path == "" || req.URL.Path == "/" {
				header.Add("Set-Cookie", "laravel_session=xyz123abc; Path=/; HttpOnly")
			}
			return &http.Response{
				StatusCode: 200,
				Header:     header,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}

	a := app.New(context.Background(), cfg)
	a.HTTPClient.Transport = app.WrapTransport(rt, a.FingerprintCache)

	m := recursion.NewManager(
		a.HTTPClient,
		cfg.Status.Exclude,
		testStaticReader{words: []string{"word1"}},
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

	resChan := m.Run(context.Background(), []string{"http://injectedrecurse.com"}, 4)
	var results []engine.Result
	for r := range resChan {
		results = append(results, r)
	}

	// Verify that injected paths themselves trigger normal wordlist recursion at depth 2 (up to maxDepth)
	// Root (1) + wordlist path (1) + 5 injected paths = 7 results at depth 1.
	// At depth 2:
	// - root/word1/word1
	// - root/.env/word1
	// - root/artisan/word1
	// - root/storage/word1
	// - root/bootstrap/word1
	// - root/vendor/word1
	// = 6 results at depth 2.
	// Total results = 13
	foundDepth2InjectedChild := false
	for _, r := range results {
		if r.Depth == 2 && strings.Contains(r.URL, ".env/word1") {
			foundDepth2InjectedChild = true
			break
		}
	}

	if !foundDepth2InjectedChild {
		t.Error("Expected wordlist children to be scanned recursively under injected paths, but none found")
	}
}

func TestHarden_SchedulerStarvation(t *testing.T) {
	cfg := config.Default()
	cfg.Recursive = true
	cfg.MaxDepth = 1

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	m := recursion.NewManager(
		client,
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
		nil,
	)

	done := make(chan struct{})
	go func() {
		resChan := m.Run(context.Background(), []string{"http://starvation.com"}, 128)
		for range resChan {
		}
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Error("Scheduler starvation/lockup under high worker count")
	}
}
