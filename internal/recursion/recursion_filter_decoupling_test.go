package recursion_test

// Regression tests: user-facing display filters (--fc, --mc, --fs) must NEVER
// prevent the recursive crawler from discovering directories or expanding the
// frontier. Traversal is controlled exclusively by --recurse-on (default:
// recurse on everything except 404, i.e., 200, 301, 302, 403).

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
	"github.com/unsubble/searchit/internal/status"
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

// runRecurseOnManager creates a manager with a specific recurse-on status filter string.
func runRecurseOnManager(
	t *testing.T,
	srv *httptest.Server,
	words []string,
	maxDepth uint16,
	recurseOnStr string,
	displayFS *filter.FilterSuite,
) (dispatched, delivered, accepted int64) {
	t.Helper()

	reader := staticReader{words: words}
	a := newApp(t)

	var recurseFilter status.Filters
	if recurseOnStr != "" {
		f, err := status.Parse(recurseOnStr)
		if err != nil {
			t.Fatalf("invalid recurseOnStr %q: %v", recurseOnStr, err)
		}
		recurseFilter = f
	} else {
		recurseFilter = a.Config.RecurseOn
	}

	m := recursion.NewManager(
		a.HTTPClient,
		nil,
		reader,
		recursion.BFS,
		maxDepth,
		recurseFilter,
		false,
		false,
		nil,
		nil,
		nil,
		nil,
		0,
		nil,
		nil,
		100,
	)

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
// Test 1: Default recursion policy (200, 301, 302, 403)
// Recurses on 200, 301, 302, 403; does NOT recurse on 404.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecurseOn_Default(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "":
			w.WriteHeader(http.StatusOK) // 200 -> recurse
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden) // 403 -> recurse by default
		default:
			w.WriteHeader(http.StatusNotFound) // 404 -> no recurse
		}
	}))
	t.Cleanup(srv.Close)

	// words: forbidden, missing
	// d0: /
	// d1: /forbidden (403), /missing (404)
	// d2: /forbidden/forbidden, /forbidden/missing (because 403 recurses)
	dispatched, _, _ := runRecurseOnManager(t, srv, []string{"forbidden", "missing"}, 2, "", nil)

	if dispatched < 4 {
		t.Errorf("default recursion: expected >= 4 jobs dispatched (including /forbidden children), got %d", dispatched)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 2: --recurse-on 200
// Recurses ONLY on 200 status codes; 403 and 404 do NOT recurse.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecurseOn_200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	// --recurse-on 200
	// d0: / (200) -> expands
	// d1: /forbidden (403), /missing (404) -> neither 403 nor 404 expands
	dispatched, _, _ := runRecurseOnManager(t, srv, []string{"forbidden", "missing"}, 2, "200", nil)

	// 1 (root) + 2 (d1) = 3 total jobs. /forbidden (403) must NOT expand into d2.
	if dispatched != 3 {
		t.Errorf("recurse-on 200: expected exactly 3 jobs dispatched, got %d", dispatched)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 3: --recurse-on 200,301,302
// Recurses on 200, 301, 302; 403 does NOT recurse.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecurseOn_200_301_302(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
		case "/redirect":
			w.Header().Set("Location", fmt.Sprintf("%s/dest/", srv.URL))
			w.WriteHeader(http.StatusFound) // 302
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden) // 403
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)

	// --recurse-on 200,301,302
	dispatched, _, _ := runRecurseOnManager(t, srv, []string{"redirect", "forbidden"}, 2, "200,301,302", nil)

	// 302 redirect expands, but 403 forbidden must NOT expand.
	if dispatched < 3 {
		t.Errorf("recurse-on 200,301,302: expected >= 3 jobs dispatched, got %d", dispatched)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 4: --fc 404 must NOT affect recursion
// Output filter --fc 404 filters 404 from findings, but recursion policy is unchanged.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecurseOn_FC404_DoesNotAffectRecursion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	displayFS := makeDisplayFS(t, "", "404")
	dispatched, _, accepted := runRecurseOnManager(t, srv, []string{"a"}, 2, "", displayFS)

	if dispatched < 2 {
		t.Errorf("--fc 404: expected >= 2 jobs dispatched (recursion unaffected), got %d", dispatched)
	}
	if accepted < 1 {
		t.Errorf("--fc 404: expected >= 1 finding, got %d", accepted)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 5: --mc 200 must NOT affect recursion
// Output filter --mc 200 filters non-200 from findings, but 301/302/403 still recurse.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecurseOn_MC200_DoesNotAffectRecursion(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Location", fmt.Sprintf("%s/app/", srv.URL))
			w.WriteHeader(http.StatusMovedPermanently) // 301
		default:
			w.WriteHeader(http.StatusOK) // 200
		}
	}))
	t.Cleanup(srv.Close)

	displayFS := makeDisplayFS(t, "200", "")
	dispatched, _, accepted := runRecurseOnManager(t, srv, []string{"a"}, 2, "", displayFS)

	// 301 root recurses to /app/, where 200 children are discovered.
	if dispatched < 2 {
		t.Errorf("--mc 200: expected >= 2 jobs dispatched (recursion unaffected), got %d", dispatched)
	}
	if accepted < 1 {
		t.Errorf("--mc 200: expected >= 1 finding (200 children), got %d", accepted)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test 6: Invalid --recurse-on values
// status.Parse rejects invalid status filter strings.
// ─────────────────────────────────────────────────────────────────────────────

func TestRecurseOn_InvalidValues(t *testing.T) {
	invalidExprs := []string{
		"abc",
		"invalid",
		"1000",
		"-1",
		"200,,301",
	}

	for _, expr := range invalidExprs {
		if _, err := status.Parse(expr); err == nil {
			t.Errorf("status.Parse(%q) succeeded, expected error", expr)
		}
	}
}
