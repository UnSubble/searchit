package recursion_test

// Regression tests: user-facing display filters (--fc, --mc, --fs) must NEVER
// prevent the recursive crawler from discovering directories or expanding the
// frontier.  These tests cover the bug where SetFilterSuite() incorrectly
// overwrote the crawl/traversal filter (m.fs) with the user-facing display
// filter, causing recursion to terminate as soon as any response was rejected
// by the display filter.
//
// Root cause (fixed):
//   SetFilterSuite now stores the display filter in m.displayFS instead of
//   overwriting m.fs.  handleResult re-evaluates the result against m.displayFS
//   (for header-level filters only) before calling onResult, keeping the crawl
//   decision independent of user-facing output filters.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/stats"
)

// makeDisplayFS constructs a *filter.FilterSuite from user-flag strings.
// mc = match-status, fc = filter-status; empty string = no constraint.
func makeDisplayFS(t *testing.T, mc, fc string) *filter.FilterSuite {
	t.Helper()
	fs, err := filter.NewFilterSuite(mc, fc, "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("makeDisplayFS(%q, %q): %v", mc, fc, err)
	}
	return fs
}

// runDecouplingManager creates a manager, optionally attaches a display filter,
// runs the scan, and returns:
//   - dispatched: total jobs sent to workers (= Candidates in summary)
//   - delivered:  total onResult calls
//   - accepted:   onResult calls with r.Accepted == true (= Findings)
func runDecouplingManager(
	t *testing.T,
	srv *httptest.Server,
	words []string,
	maxDepth uint16,
	displayFS *filter.FilterSuite,
) (dispatched, delivered, accepted int64) {
	t.Helper()

	reader := staticReader{words: words}
	m := newManager(t, reader, recursion.BFS, maxDepth)

	if displayFS != nil {
		m.SetFilterSuite(displayFS)
	}

	c := stats.NewCollector()
	m.SetStats(c)

	var del, acc int64
	m.Run(context.Background(), context.Background(),
		[]string{srv.URL + "/"},
		4,
		func(r engine.Result) {
			atomic.AddInt64(&del, 1)
			if r.Accepted {
				atomic.AddInt64(&acc, 1)
			}
		},
	)

	snap := c.Snapshot()
	return snap.JobsProduced, del, acc
}

// ─────────────────────────────────────────────────────────────────────────────
// Test a — recursive scan, no filters
// Baseline: all paths return 200; recursion generates candidates at all depths.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecursionDecoupling_NoFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// wordlist ["a"], maxDepth=2: root (d0) → /a (d1) → /a/a (d2)
	dispatched, _, accepted := runDecouplingManager(t, srv, []string{"a"}, 2, nil)

	if dispatched < 2 {
		t.Errorf("no filter: expected >= 2 jobs dispatched, got %d", dispatched)
	}
	if accepted < 1 {
		t.Errorf("no filter: expected >= 1 finding, got %d", accepted)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test b — recursive scan, --fc 404
// 404 is the default crawl-exclusion too; recursion on 200 paths continues.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecursionDecoupling_FC404(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(http.StatusOK) // all paths 200
	}))
	t.Cleanup(srv.Close)

	// Display filter: --fc 404 (filter 404 from output)
	fs := makeDisplayFS(t, "", "404")
	dispatched, _, accepted := runDecouplingManager(t, srv, []string{"a"}, 2, fs)

	// 200-only server; display filter rejects nothing here.
	// Key invariant: crawl continues normally.
	if dispatched < 2 {
		t.Errorf("fc=404: expected >= 2 jobs dispatched, got %d", dispatched)
	}
	if accepted < 1 {
		t.Errorf("fc=404: expected >= 1 finding, got %d", accepted)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test c — recursive scan, --fc 3xx,4xx (the primary regression case)
//
// Server layout:
//   /           → 302 redirect to /dest/   (root itself is 3xx)
//   /dest/      → 200
//   /dest/a     → 200
//
// With the bug:
//   Root 302 is rejected by the display filter (now used as crawl filter) →
//   Accepted=false → handleResult returns early → no directory generator →
//   dispatched=1, recursion dead.
//
// With the fix:
//   Root 302 passes the traversal filter (m.fs) → handleResult enqueues /dest/ →
//   /dest/ 200 triggers directory expansion → dispatched > 1.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecursionDecoupling_FC3xx4xx(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "":
			// Root redirects to /dest/ — status 302, not followed by the scanner
			// by default (FollowRedirects: false in config.Default()).
			w.Header().Set("Location", fmt.Sprintf("%s/dest/", srv.URL))
			w.WriteHeader(http.StatusFound) // 302
		default:
			w.WriteHeader(http.StatusOK) // 200 for everything else
		}
	}))
	t.Cleanup(srv.Close)

	// Display filter: --fc 3xx,4xx (the user does not want redirect/not-found
	// results in the output, but recursion must continue through redirects).
	fs := makeDisplayFS(t, "", "3xx,4xx")

	dispatched, _, accepted := runDecouplingManager(t, srv, []string{"a"}, 2, fs)

	// Regression check: the redirect at root must trigger recursion.
	// Without the fix, dispatched == 1 (only root, no recursion).
	if dispatched < 2 {
		t.Errorf(
			"fc=3xx,4xx regression: expected >= 2 jobs dispatched (recursion through redirect), got %d\n"+
				"This indicates the display filter is incorrectly blocking recursion.",
			dispatched,
		)
	}
	// The 302 root must NOT appear as a finding (display filter rejects it).
	// /dest/ and children (200) should be accepted.
	if accepted == 0 {
		t.Errorf("fc=3xx,4xx: expected >= 1 finding (200 results), got 0")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test d — recursive scan, --mc 200
//
// Only 200 responses are reported as findings. But the root returns 301 and
// must still trigger recursion so that 200 children are discovered.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecursionDecoupling_MC200(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "":
			// Root returns 301 Moved Permanently to /home/
			w.Header().Set("Location", fmt.Sprintf("%s/home/", srv.URL))
			w.WriteHeader(http.StatusMovedPermanently) // 301
		default:
			w.WriteHeader(http.StatusOK) // 200
		}
	}))
	t.Cleanup(srv.Close)

	// Display filter: --mc 200 (only report 200 responses)
	fs := makeDisplayFS(t, "200", "")

	dispatched, _, accepted := runDecouplingManager(t, srv, []string{"a"}, 2, fs)

	// Regression: root 301 must trigger recursion even though it doesn't pass mc=200.
	if dispatched < 2 {
		t.Errorf(
			"mc=200 regression: expected >= 2 jobs dispatched (recursion through 301), got %d\n"+
				"This indicates --mc is incorrectly blocking recursion.",
			dispatched,
		)
	}
	// Children (/home/*, etc.) return 200 and must appear as findings.
	if accepted == 0 {
		t.Errorf("mc=200: expected >= 1 finding (200 children), got 0")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test e — redirecting root URL, no display filter
//
// Verifies baseline behaviour: root 302 → redirect handling → recursion.
// This is the un-filtered analogue of test c.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecursionDecoupling_RedirectingRoot_NoFilter(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "" {
			w.Header().Set("Location", fmt.Sprintf("%s/app/", srv.URL))
			w.WriteHeader(http.StatusFound) // 302
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// No display filter — should behave like baseline
	dispatched, _, _ := runDecouplingManager(t, srv, []string{"index", "admin"}, 1, nil)

	if dispatched < 2 {
		t.Errorf("redirect root (no filter): expected >= 2 jobs dispatched, got %d", dispatched)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test f — display filter that rejects ALL responses must not kill recursion
//
// This edge case proves that even a maximally restrictive display filter
// (--fc 1xx,2xx,3xx,4xx,5xx, i.e. reject everything) doesn't prevent the
// crawler from generating candidates. Findings will be 0 but dispatched > 1.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecursionDecoupling_RejectAllDisplayFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// Display filter rejects every status code.
	fs := makeDisplayFS(t, "", "1xx,2xx,3xx,4xx,5xx")

	dispatched, _, accepted := runDecouplingManager(t, srv, []string{"a"}, 2, fs)

	// Crawl must proceed normally — all candidates generated.
	if dispatched < 2 {
		t.Errorf(
			"reject-all display filter: expected >= 2 jobs dispatched, got %d\n"+
				"Display filter must never block crawl candidate generation.",
			dispatched,
		)
	}
	// No findings expected since display filter rejects everything.
	if accepted != 0 {
		t.Errorf("reject-all display filter: expected 0 findings, got %d", accepted)
	}
}
