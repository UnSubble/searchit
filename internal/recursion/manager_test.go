package recursion_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/wordlist"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func newApp(t *testing.T) *app.App {
	t.Helper()
	return app.New(context.Background(), config.Default())
}

// staticReader streams a fixed set of words, simulating a wordlist without disk I/O.
type staticReader struct {
	words []string
}

func (r staticReader) Read(ctx context.Context, out chan<- string) error {
	for _, w := range r.words {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- w:
		}
	}
	return nil
}

func collectResults(ch <-chan engine.Result) []engine.Result {
	var out []engine.Result
	for r := range ch {
		out = append(out, r)
	}
	return out
}

func newManager(t *testing.T, reader wordlist.Reader, strategy recursion.Strategy, maxDepth uint16) *recursion.Manager {
	a := newApp(t)
	return recursion.NewManager(
		a.HTTPClient,
		a.Config.Status.Exclude,
		reader,
		strategy,
		maxDepth,
		a.Config.RecurseOn,
		a.Config.Paths.NormalizePaths,
		a.Config.Paths.CollapseSlashes,
		nil,
		nil,
		nil,
		nil,
	)
}

func TestParseStrategy(t *testing.T) {
	tests := []struct {
		input   string
		want    recursion.Strategy
		wantErr bool
	}{
		{"bfs", recursion.BFS, false},
		{"BFS", recursion.BFS, false},
		{"dfs", recursion.DFS, false},
		{"DFS", recursion.DFS, false},
		{" Bfs ", recursion.BFS, false},
		{"invalid", recursion.BFS, true},
		{"", recursion.BFS, true},
	}

	for _, tc := range tests {
		got, err := recursion.ParseStrategy(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseStrategy(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
		}
		if err == nil && got != tc.want {
			t.Errorf("ParseStrategy(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestFrontier_BFS_Order(t *testing.T) {
	f := recursion.NewFrontier(recursion.BFS)
	jobs := []engine.Job{{URL: "a"}, {URL: "b"}, {URL: "c"}}
	for _, j := range jobs {
		f.Push(j)
	}

	for _, want := range jobs {
		got, ok := f.Pop()
		if !ok {
			t.Fatal("Pop returned false, want a job")
		}
		if got.URL != want.URL {
			t.Errorf("Pop() = %q, want %q (BFS must preserve insertion order)", got.URL, want.URL)
		}
	}
}

func TestFrontier_DFS_Order(t *testing.T) {
	f := recursion.NewFrontier(recursion.DFS)
	jobs := []engine.Job{{URL: "a"}, {URL: "b"}, {URL: "c"}}
	for _, j := range jobs {
		f.Push(j)
	}

	// DFS pushes to front, so the last pushed comes out first.
	want := []string{"c", "b", "a"}
	for _, w := range want {
		got, ok := f.Pop()
		if !ok {
			t.Fatal("Pop returned false, want a job")
		}
		if got.URL != w {
			t.Errorf("Pop() = %q, want %q (DFS must reverse insertion order)", got.URL, w)
		}
	}
}

func TestFrontier_Len(t *testing.T) {
	f := recursion.NewFrontier(recursion.BFS)
	if f.Len() != 0 {
		t.Errorf("Len() = %d, want 0 on empty frontier", f.Len())
	}

	f.Push(engine.Job{URL: "x"})
	f.Push(engine.Job{URL: "y"})
	if f.Len() != 2 {
		t.Errorf("Len() = %d, want 2", f.Len())
	}

	f.Pop()
	if f.Len() != 1 {
		t.Errorf("Len() = %d, want 1 after Pop", f.Len())
	}
}

func TestFrontier_Pop_EmptyReturnsFalse(t *testing.T) {
	f := recursion.NewFrontier(recursion.BFS)
	_, ok := f.Pop()
	if ok {
		t.Error("Pop on empty frontier returned true, want false")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ShouldRecurse
// ─────────────────────────────────────────────────────────────────────────────

func TestDefaultRecursePolicy(t *testing.T) {
	cfg := config.Default()
	yes := []int{200, 301, 302, 403}
	for _, code := range yes {
		if !cfg.RecurseOn.Match(code) {
			t.Errorf("default RecurseOn.Match(%d) = false, want true", code)
		}
	}

	no := []int{100, 204, 400, 404, 500, 503}
	for _, code := range no {
		if cfg.RecurseOn.Match(code) {
			t.Errorf("default RecurseOn.Match(%d) = true, want false", code)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Manager
// ─────────────────────────────────────────────────────────────────────────────

// singleDepthServer returns 200 for every path at depth 0 and 404 for all others,
// letting tests control exactly which URLs trigger recursion.
func respondWith(codes map[string]int, fallback int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if code, ok := codes[r.URL.Path]; ok {
			w.WriteHeader(code)
			return
		}
		w.WriteHeader(fallback)
	}
}

func TestManager_VisitedURLsNotRevisited(t *testing.T) {
	mu := sync.Mutex{}
	hits := map[string]int{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// Both seeds point to the same logical URL (one has trailing slash).
	seeds := []string{srv.URL + "/admin", srv.URL + "/admin/"}
	reader := staticReader{words: []string{"x"}}

	m := newManager(t, reader, recursion.BFS, 0)
	results := collectResults(m.Run(context.Background(), seeds, 4))

	// maxDepth=0 means no recursion; we only expect the seeds themselves.
	// /admin and /admin/ are the same canonical URL, so only one should be fetched.
	if len(results) != 1 {
		t.Errorf("got %d results, want 1 (duplicate seed must be deduped)", len(results))
	}
}

func TestManager_MaxDepthEnforced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	seeds := []string{srv.URL + "/start"}
	reader := staticReader{words: []string{"child"}}

	// maxDepth=1: seed (depth 0) recurses, but children (depth 1) do not.
	m := newManager(t, reader, recursion.BFS, 1)
	results := collectResults(m.Run(context.Background(), seeds, 4))

	for _, r := range results {
		if r.Depth > 1 {
			t.Errorf("result at depth %d exceeds maxDepth 1: %s", r.Depth, r.URL)
		}
	}
}

func TestManager_NonRecursingStatusSkipped(t *testing.T) {
	srv := httptest.NewServer(respondWith(
		map[string]int{"/start": 200, "/start/child": 404},
		404,
	))
	t.Cleanup(srv.Close)

	seeds := []string{srv.URL + "/start"}
	// child would only appear if /start/child triggered further recursion,
	// but 404 must not recurse.
	reader := staticReader{words: []string{"child", "grandchild"}}

	m := newManager(t, reader, recursion.BFS, 3)
	results := collectResults(m.Run(context.Background(), seeds, 4))

	// Default config excludes 404; only /start (200) should appear.
	for _, r := range results {
		if r.Depth > 1 {
			t.Errorf("recursion beyond 404 result: depth=%d url=%s", r.Depth, r.URL)
		}
	}
}

func TestManager_NoDuplicateJobsScheduled(t *testing.T) {
	mu := sync.Mutex{}
	hits := map[string]int{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// Wordlist has duplicates; the visited set must prevent double-scheduling.
	seeds := []string{srv.URL + "/a"}
	reader := staticReader{words: []string{"dup", "dup", "dup"}}

	m := newManager(t, reader, recursion.BFS, 1)
	collectResults(m.Run(context.Background(), seeds, 4))

	mu.Lock()
	defer mu.Unlock()
	for path, count := range hits {
		if count > 1 {
			t.Errorf("path %q fetched %d times, want 1", path, count)
		}
	}
}

func TestManager_BFS_TraversalOrder(t *testing.T) {
	// Serve all paths so every job produces a result that could recurse.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// Three seeds at depth 0; each gets one child at depth 1.
	seeds := []string{
		fmt.Sprintf("%s/a", srv.URL),
		fmt.Sprintf("%s/b", srv.URL),
		fmt.Sprintf("%s/c", srv.URL),
	}
	reader := staticReader{words: []string{"leaf"}}

	m := newManager(t, reader, recursion.BFS, 1)
	results := collectResults(m.Run(context.Background(), seeds, 1))

	depth0, depth1 := 0, 0
	for _, r := range results {
		switch r.Depth {
		case 0:
			depth0++
		case 1:
			depth1++
		}
	}
	if depth0 != 3 {
		t.Errorf("BFS: got %d depth-0 results, want 3", depth0)
	}
	if depth1 != 3 {
		t.Errorf("BFS: got %d depth-1 results, want 3", depth1)
	}
}

func TestManager_DFS_TraversalOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	seeds := []string{fmt.Sprintf("%s/root", srv.URL)}
	reader := staticReader{words: []string{"child"}}

	m := newManager(t, reader, recursion.DFS, 2)
	results := collectResults(m.Run(context.Background(), seeds, 1))

	// With DFS and a single worker, /root/child must be visited before any
	// sibling of /root because DFS inserts to the front.
	depths := make([]int, len(results))
	for i, r := range results {
		depths[i] = int(r.Depth)
	}

	// DFS: depth should strictly increase before decreasing — we go deep first.
	// Check at least that a depth-2 result exists (DFS went into the subtree).
	maxDepth := 0
	for _, d := range depths {
		if d > maxDepth {
			maxDepth = d
		}
	}
	if maxDepth < 2 {
		t.Errorf("DFS did not reach depth 2; max depth seen = %d", maxDepth)
	}
}

func TestManager_CleanShutdown_Cancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	seeds := make([]string, 50)
	for i := range seeds {
		seeds[i] = fmt.Sprintf("%s/%d", srv.URL, i)
	}
	reader := staticReader{words: []string{"a", "b", "c"}}

	ctx, cancel := context.WithCancel(context.Background())

	m := newManager(t, reader, recursion.BFS, 3)
	ch := m.Run(ctx, seeds, 8)

	// Cancel after seeing the first result; the channel must still close cleanly.
	<-ch
	cancel()

	// Drain; must not deadlock.
	for range ch {
	}
}

func TestManager_EmptySeeds(t *testing.T) {
	m := newManager(t, staticReader{}, recursion.BFS, 1)
	results := collectResults(m.Run(context.Background(), nil, 4))
	if len(results) != 0 {
		t.Errorf("got %d results for empty seed list, want 0", len(results))
	}
}

func TestManager_ResultsContainAllDepths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	seeds := []string{srv.URL + "/root"}
	reader := staticReader{words: []string{"a"}}

	m := newManager(t, reader, recursion.BFS, 2)
	results := collectResults(m.Run(context.Background(), seeds, 2))

	seen := map[uint16]bool{}
	for _, r := range results {
		seen[r.Depth] = true
	}

	for _, d := range []uint16{0, 1, 2} {
		if !seen[d] {
			t.Errorf("no result at depth %d", d)
		}
	}

	// Verify URLs are sorted by checking that all URLs are unique.
	urls := make([]string, len(results))
	for i, r := range results {
		urls[i] = r.URL
	}
	sort.Strings(urls)
	for i := 1; i < len(urls); i++ {
		if urls[i] == urls[i-1] {
			t.Errorf("duplicate URL in results: %q", urls[i])
		}
	}
}

func TestManager_CustomRecursionPolicy(t *testing.T) {
	srv := httptest.NewServer(respondWith(
		map[string]int{"/start": 200, "/start/a": 201},
		404,
	))
	t.Cleanup(srv.Close)

	seeds := []string{srv.URL + "/start"}
	reader := staticReader{words: []string{"a", "b"}}

	a := newApp(t)
	recurseOn := status.MustParse("201")
	m := recursion.NewManager(a.HTTPClient, a.Config.Status.Exclude, reader, recursion.BFS, 2, recurseOn, false, false, nil, nil, nil, nil)

	results := collectResults(m.Run(context.Background(), seeds, 2))

	for _, r := range results {
		if r.Depth > 0 {
			t.Errorf("expected no recursion, but got result at depth %d: %s", r.Depth, r.URL)
		}
	}
}

func TestManager_CustomRecursionPolicy_Matches(t *testing.T) {
	srv := httptest.NewServer(respondWith(
		map[string]int{"/start": 201, "/start/a": 200},
		404,
	))
	t.Cleanup(srv.Close)

	seeds := []string{srv.URL + "/start"}
	reader := staticReader{words: []string{"a"}}

	a := newApp(t)
	recurseOn := status.MustParse("201")
	m := recursion.NewManager(a.HTTPClient, a.Config.Status.Exclude, reader, recursion.BFS, 2, recurseOn, false, false, nil, nil, nil, nil)

	results := collectResults(m.Run(context.Background(), seeds, 2))

	foundChild := false
	for _, r := range results {
		if r.Depth == 1 && strings.HasSuffix(r.URL, "/start/a") {
			foundChild = true
		}
		if r.Depth > 1 {
			t.Errorf("expected max depth 1 child to not recurse, but got depth %d: %s", r.Depth, r.URL)
		}
	}
	if !foundChild {
		t.Error("expected child /start/a to be discovered via custom recursion status 201")
	}
}

func TestManager_RecursionDepthBoundaries(t *testing.T) {
	srv := httptest.NewServer(respondWith(
		map[string]int{
			"/start":         200,
			"/start/a":       200,
			"/start/a/b":     200,
			"/start/a/b/c":   200,
			"/start/a/b/c/d": 200,
		},
		404,
	))
	t.Cleanup(srv.Close)

	seeds := []string{srv.URL + "/start"}
	reader := staticReader{words: []string{"a", "b", "c", "d"}}

	t.Run("max depth 2 limits traversal", func(t *testing.T) {
		a := newApp(t)
		m := recursion.NewManager(
			a.HTTPClient,
			a.Config.Status.Exclude,
			reader,
			recursion.BFS,
			2,
			a.Config.RecurseOn,
			false,
			false,
			nil,
			nil,
			nil,
			nil,
		)

		results := collectResults(m.Run(context.Background(), seeds, 2))

		var maxDepthReached uint16
		for _, r := range results {
			if r.Depth > maxDepthReached {
				maxDepthReached = r.Depth
			}
		}

		if maxDepthReached > 2 {
			t.Errorf("expected max depth 2, but traversed to depth %d", maxDepthReached)
		}
	})
}

func TestStrategy_String(t *testing.T) {
	tests := []struct {
		strat recursion.Strategy
		want  string
	}{
		{recursion.BFS, "bfs"},
		{recursion.DFS, "dfs"},
		{recursion.Strategy(-1), "unknown"},
		{recursion.Strategy(999), "unknown"},
	}

	for _, tc := range tests {
		if got := tc.strat.String(); got != tc.want {
			t.Errorf("%d.String() = %q, want %q", int(tc.strat), got, tc.want)
		}
	}
}

func TestFrontier_Peek_Empty(t *testing.T) {
	f := recursion.NewFrontier(recursion.BFS)
	_, ok := f.Peek()
	if ok {
		t.Error("Peek on empty frontier returned true, want false")
	}
}

func TestFrontier_Grow_BFS_DFS(t *testing.T) {
	t.Run("BFS grow", func(t *testing.T) {
		f := recursion.NewFrontier(recursion.BFS)
		// Push more than default buffer capacity (2048) to trigger grow
		numJobs := 2500
		for i := 0; i < numJobs; i++ {
			f.Push(engine.Job{URL: fmt.Sprintf("url-%d", i)})
		}

		if f.Len() != numJobs {
			t.Fatalf("expected length %d, got %d", numJobs, f.Len())
		}

		for i := 0; i < numJobs; i++ {
			job, ok := f.Pop()
			if !ok {
				t.Fatalf("Pop failed at index %d", i)
			}
			expectedURL := fmt.Sprintf("url-%d", i)
			if job.URL != expectedURL {
				t.Errorf("expected URL %q, got %q", expectedURL, job.URL)
			}
		}
	})

	t.Run("DFS grow", func(t *testing.T) {
		f := recursion.NewFrontier(recursion.DFS)
		numJobs := 2500
		for i := 0; i < numJobs; i++ {
			f.Push(engine.Job{URL: fmt.Sprintf("url-%d", i)})
		}

		if f.Len() != numJobs {
			t.Fatalf("expected length %d, got %d", numJobs, f.Len())
		}

		// DFS reverses order
		for i := numJobs - 1; i >= 0; i-- {
			job, ok := f.Pop()
			if !ok {
				t.Fatalf("Pop failed at index %d", i)
			}
			expectedURL := fmt.Sprintf("url-%d", i)
			if job.URL != expectedURL {
				t.Errorf("expected URL %q, got %q", expectedURL, job.URL)
			}
		}
	})
}

type errReader struct {
	err error
}

func (r errReader) Read(ctx context.Context, out chan<- string) error {
	return r.err
}

func TestManager_ReaderErrorHandling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	seeds := []string{srv.URL + "/start"}
	r := errReader{err: fmt.Errorf("read error simulated")}

	a := newApp(t)
	// MaxDepth is 2 so it wants to recurse on /start (depth 0 -> depth 1)
	m := recursion.NewManager(
		a.HTTPClient,
		a.Config.Status.Exclude,
		r,
		recursion.BFS,
		2,
		a.Config.RecurseOn,
		false,
		false,
		nil,
		nil,
		nil,
		nil,
	)

	results := collectResults(m.Run(context.Background(), seeds, 2))
	// We only expect the seed (depth 0) to be returned since reader fails during handleResult.
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d: %v", len(results), results)
	}
}

func TestManager_Run_ImmediateCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	seeds := []string{srv.URL + "/start"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	m := newManager(t, staticReader{words: []string{"a"}}, recursion.BFS, 2)
	results := collectResults(m.Run(ctx, seeds, 2))

	// Should drain and return immediately without running
	if len(results) > 0 {
		t.Errorf("expected 0 results due to immediate cancellation, got %v", results)
	}
}
