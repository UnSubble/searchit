package recursion_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
)

func TestRecursion_OpenEndedDeterministicAccounting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/admin" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	collector := stats.NewCollector()
	wordlist := []string{"admin", "login", "api", "test", "docs"}
	entriesPerDir := int64(len(wordlist))
	maxDepth := uint16(2)

	fs, _ := filter.NewFilterSuite("200", "404", "", "", nil, nil, nil, nil)

	manager := recursion.NewManager(
		srv.Client(),
		status.MustParse("404"),
		acctStaticReader{words: wordlist},
		recursion.BFS,
		maxDepth,
		status.MustParse("200"),
		false,
		false,
		nil, nil, nil, nil, 0, nil, nil,
		entriesPerDir,
	)
	manager.SetDisableWildcard(true)
	manager.SetFilterSuite(fs)
	manager.SetStats(collector)

	ctx := context.Background()

	manager.Run(ctx, ctx, []string{srv.URL}, 2, func(r engine.Result) {})

	snap := collector.Snapshot()
	if snap.IsFinite {
		t.Errorf("expected recursive scan to set IsFinite = false, got true")
	}
	if snap.RequestsSent < 11 {
		t.Errorf("expected at least 11 requests sent, got %d", snap.RequestsSent)
	}
	if snap.DirectoriesDiscovered < 1 {
		t.Errorf("expected at least 1 discovered directory, got %d", snap.DirectoriesDiscovered)
	}
}
