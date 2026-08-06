package recursion_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
)

type acctStaticReader struct {
	words []string
}

func (r acctStaticReader) Read(ctx context.Context, out chan<- string) error {
	for _, w := range r.words {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- w:
		}
	}
	return nil
}

func TestAccounting_StandardScan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	collector := stats.NewCollector()
	wordlist := []string{"admin", "login", "api"}
	entriesPerDir := int64(len(wordlist))
	totalWork := int64(1) + entriesPerDir

	collector.SetTotalWork(totalWork)

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)

	manager := recursion.NewManager(
		srv.Client(),
		status.MustParse("404"),
		acctStaticReader{words: wordlist},
		recursion.BFS,
		1, // depth 1
		status.MustParse("200"),
		false,
		false,
		0, nil, nil,
		entriesPerDir,
	)
	manager.SetDisableWildcard(true)
	manager.SetFilterSuite(fs)
	manager.SetStats(collector)

	ctx := context.Background()
	manager.Run(ctx, ctx, []string{srv.URL}, 2, func(r engine.Result) {})

	snap := collector.Snapshot()
	if snap.TotalWork != totalWork {
		t.Errorf("TotalWork = %d, want %d", snap.TotalWork, totalWork)
	}
	if snap.Completed != totalWork {
		t.Errorf("Completed = %d, want TotalWork (%d)", snap.Completed, totalWork)
	}
	if snap.Completed != snap.Tried+snap.Skipped {
		t.Errorf("Completed (%d) != Tried (%d) + Skipped (%d)", snap.Completed, snap.Tried, snap.Skipped)
	}
	if snap.Progress != 100.0 {
		t.Errorf("Progress = %f, want 100.0", snap.Progress)
	}
}

func TestAccounting_StandardScanExt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	collector := stats.NewCollector()
	wordlist := []string{"admin", "login"}
	exts := []string{"php", "html"}
	entriesPerDir := int64(len(wordlist) * (1 + len(exts))) // 2 * 3 = 6
	totalWork := int64(1) + entriesPerDir                   // 1 seed + 6 candidates = 7

	collector.SetTotalWork(totalWork)

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)

	manager := recursion.NewManager(
		srv.Client(),
		status.MustParse("404"),
		acctStaticReader{words: wordlist},
		recursion.BFS,
		1,
		status.MustParse("200"),
		false,
		false,
		0, nil, nil,
		entriesPerDir,
	)
	manager.SetDisableWildcard(true)
	manager.SetFilterSuite(fs)
	manager.SetExtensions(exts)
	manager.SetStats(collector)

	ctx := context.Background()
	manager.Run(ctx, ctx, []string{srv.URL}, 2, func(r engine.Result) {})

	snap := collector.Snapshot()
	if snap.TotalWork != totalWork {
		t.Errorf("TotalWork = %d, want %d", snap.TotalWork, totalWork)
	}
	if snap.Completed != totalWork {
		t.Errorf("Completed = %d, want %d", snap.Completed, totalWork)
	}
	if snap.Completed != snap.Tried+snap.Skipped {
		t.Errorf("Completed (%d) != Tried (%d) + Skipped (%d)", snap.Completed, snap.Tried, snap.Skipped)
	}
}

func TestAccounting_RecursiveScan(t *testing.T) {
	// Root and /admin paths are directories (200), others 404
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || strings.Contains(r.URL.Path, "admin") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	collector := stats.NewCollector()
	wordlist := []string{"admin", "login"}
	entriesPerDir := int64(len(wordlist)) // 2
	maxDepth := uint16(2)
	totalWork := int64(1) + entriesPerDir*int64(maxDepth) // 1 seed + 2 * 2 = 5

	collector.SetTotalWork(totalWork)

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
		0, nil, nil,
		entriesPerDir,
	)
	manager.SetDisableWildcard(true)
	manager.SetFilterSuite(fs)
	manager.SetStats(collector)

	ctx := context.Background()
	manager.Run(ctx, ctx, []string{srv.URL}, 2, func(r engine.Result) {})

	snap := collector.Snapshot()
	if snap.TotalWork != totalWork {
		t.Errorf("TotalWork = %d, want %d", snap.TotalWork, totalWork)
	}
	if snap.Completed != totalWork {
		t.Errorf("Completed = %d, want %d", snap.Completed, totalWork)
	}
	if snap.Completed != snap.Tried+snap.Skipped {
		t.Errorf("Completed (%d) != Tried (%d) + Skipped (%d)", snap.Completed, snap.Tried, snap.Skipped)
	}
}

func TestAccounting_RootExcluded(t *testing.T) {
	// Root URL and all paths return status 404 (excluded by filter suite)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	collector := stats.NewCollector()
	wordlist := []string{"admin", "login"}
	entriesPerDir := int64(len(wordlist)) // 2
	maxDepth := uint16(3)
	totalWork := int64(1) + entriesPerDir*int64(maxDepth-1) // 1 + 4 = 5

	collector.SetTotalWork(totalWork)

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
		0, nil, nil,
		entriesPerDir,
	)
	manager.SetDisableWildcard(true)
	manager.SetFilterSuite(fs)
	manager.SetStats(collector)

	ctx := context.Background()
	manager.Run(ctx, ctx, []string{srv.URL}, 2, func(r engine.Result) {})

	snap := collector.Snapshot()
	if snap.Completed != snap.Tried+snap.Skipped {
		t.Errorf("Completed (%d) != Tried (%d) + Skipped (%d)", snap.Completed, snap.Tried, snap.Skipped)
	}
	if snap.Completed != totalWork {
		t.Errorf("Completed (%d) != TotalWork (%d) on root exclusion", snap.Completed, totalWork)
	}
}

func TestAccounting_CancellationGracefulAndImmediate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	collector := stats.NewCollector()
	wordlist := make([]string, 50)
	for i := 0; i < 50; i++ {
		wordlist[i] = "item"
	}
	entriesPerDir := int64(len(wordlist))
	maxDepth := uint16(2)
	totalWork := int64(1) + entriesPerDir*int64(maxDepth) // 101

	collector.SetTotalWork(totalWork)

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
		0, nil, nil,
		entriesPerDir,
	)
	manager.SetDisableWildcard(true)
	manager.SetFilterSuite(fs)
	manager.SetStats(collector)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after short delay
	go func() {
		time.Sleep(15 * time.Millisecond)
		cancel()
	}()

	manager.Run(ctx, ctx, []string{srv.URL}, 2, func(r engine.Result) {})

	snap := collector.Snapshot()
	if snap.TotalWork != totalWork {
		t.Errorf("TotalWork = %d, want %d", snap.TotalWork, totalWork)
	}
	if snap.Completed > totalWork {
		t.Errorf("Completed (%d) > TotalWork (%d) on cancellation!", snap.Completed, totalWork)
	}
	if snap.Completed != snap.Tried+snap.Skipped {
		t.Errorf("Completed (%d) != Tried (%d) + Skipped (%d)", snap.Completed, snap.Tried, snap.Skipped)
	}
}

func TestAccounting_FuzzMultiPlaceholder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	collector := stats.NewCollector()
	fooWords := []string{"admin", "test", "other"}
	barWords := []string{"users", "data"}

	fs, _ := filter.NewFilterSuite("200", "404", "", "", nil, nil, nil, nil)

	runner := &fuzz.Runner{
		TargetURL: srv.URL + "/FOO/BAR",
		FooWords:  fooWords,
		BarWords:  barWords,
		Client:    srv.Client(),
		FS:        fs,
		Threads:   2,
		Collector: collector,
	}

	totalWork := runner.EstimateCandidates(0) // 3 * 2 = 6
	collector.SetTotalWork(totalWork)

	ctx := context.Background()
	err := runner.Run(ctx, ctx, "bfs", nil, func(res fuzz.Result) {})
	if err != nil {
		t.Fatalf("fuzz runner failed: %v", err)
	}

	snap := collector.Snapshot()
	if snap.TotalWork != totalWork {
		t.Errorf("TotalWork = %d, want %d", snap.TotalWork, totalWork)
	}
	if snap.Completed != totalWork {
		t.Errorf("Completed (%d) != TotalWork (%d) in fuzzing!", snap.Completed, totalWork)
	}
	if snap.Completed != snap.Tried+snap.Skipped {
		t.Errorf("Completed (%d) != Tried (%d) + Skipped (%d)", snap.Completed, snap.Tried, snap.Skipped)
	}
}
