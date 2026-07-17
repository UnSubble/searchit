package recursion_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/profile"
	scanProfile "github.com/unsubble/searchit/internal/profile/scan"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
)

func init() {
	_ = profile.RegisterBuiltinValidators()
	_ = profile.RegisterBuiltinDecoders()
}

// levelConfig controls workload scale per benchmark level.
type levelConfig struct {
	targetCount  int
	wordlistSize int
	workerCounts []int
	maxDepth     uint16
	runs         int
}

func getBenchmarkLevel() int {
	val := os.Getenv("BENCHMARK_LEVEL")
	if val == "" {
		return 1 // default to LEVEL-1 Smoke
	}
	lvl, err := strconv.Atoi(val)
	if err != nil || lvl < 1 || lvl > 5 {
		return 1
	}
	return lvl
}

func getLevelConfig(lvl int) levelConfig {
	switch lvl {
	case 1: // LEVEL-1: Smoke (< 30s)
		return levelConfig{
			targetCount:  5,
			wordlistSize: 5,
			workerCounts: []int{1, 4},
			maxDepth:     1,
			runs:         1,
		}
	case 2: // LEVEL-2: Test (< 5m)
		return levelConfig{
			targetCount:  20,
			wordlistSize: 15,
			workerCounts: []int{1, 2, 4, 16},
			maxDepth:     2,
			runs:         1,
		}
	case 3: // LEVEL-3: Hardening (< 15m)
		return levelConfig{
			targetCount:  100,
			wordlistSize: 50,
			workerCounts: []int{1, 4, 16, 64},
			maxDepth:     2,
			runs:         1,
		}
	case 4: // LEVEL-4: Benchmark (< 60m)
		return levelConfig{
			targetCount:  500,
			wordlistSize: 200,
			workerCounts: []int{1, 4, 32, 128},
			maxDepth:     3,
			runs:         1,
		}
	case 5: // LEVEL-5: Rigorous (unrestricted)
		return levelConfig{
			targetCount:  1000,
			wordlistSize: 500,
			workerCounts: []int{1, 2, 4, 8, 16, 32, 64, 128, 256},
			maxDepth:     3,
			runs:         3,
		}
	default:
		return getLevelConfig(1)
	}
}

func reportDeterminismFailure(t *testing.T, w1Count, wNCount int, wN int) {
	diff := w1Count - wNCount
	if diff < 0 {
		diff = -diff
	}
	t.Errorf(`FAILED

Determinism Validation

worker=1:
%d URLs

worker=%d:
%d URLs

difference:
%d URLs

status:
RELEASE BLOCKER`, w1Count, wN, wNCount, diff)
}

func sortResults(res []engine.Result) {
	sort.Slice(res, func(i, j int) bool {
		if res[i].URL != res[j].URL {
			return res[i].URL < res[j].URL
		}
		return res[i].StatusCode < res[j].StatusCode
	})
}

func testFormatters(t *testing.T, w1Results, wNResults []engine.Result, wN int) {
	formats := []output.Format{
		output.FormatText,
		output.FormatJSON,
		output.FormatNDJSON,
		output.FormatCSV,
		output.FormatMarkdown,
	}

	for _, fmtName := range formats {
		var buf1 bytes.Buffer
		f1 := output.New(fmtName, &buf1, false, false, false)
		for _, r := range w1Results {
			_ = f1.Print(r)
		}
		_ = f1.Close()

		var bufN bytes.Buffer
		fN := output.New(fmtName, &bufN, false, false, false)
		for _, r := range wNResults {
			_ = fN.Print(r)
		}
		_ = fN.Close()

		if buf1.String() != bufN.String() {
			t.Errorf(`FAILED

Formatter Determinism Validation

format:
%s

worker=1 output length:
%d

worker=%d output length:
%d

status:
RELEASE BLOCKER`, fmtName, buf1.Len(), wN, bufN.Len())
		}
	}
}

// testStaticReader simulates a wordlist.
type testHardeningStaticReader struct {
	words []string
}

func (r testHardeningStaticReader) Read(ctx context.Context, out chan<- string) error {
	for _, w := range r.words {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- w:
		}
	}
	return nil
}

func runDeterminismTest(
	t *testing.T,
	seeds []string,
	reader testHardeningStaticReader,
	maxDepth uint16,
	strategy recursion.Strategy,
	setupClient func(*http.Client),
) []engine.Result {
	client := &http.Client{}
	if setupClient != nil {
		setupClient(client)
	}

	excludeFilters, _ := status.Parse("404")
	recurseOnFilters, _ := status.Parse("200,301,302,403")

	// Get level configuration
	level := getBenchmarkLevel()
	lcfg := getLevelConfig(level)

	// Reset GlobalTelemetry record
	stats.GlobalInstrumentation.Reset()
	atomic.StoreInt32(&stats.GlobalInstrumentation.Enabled, 1)

	// Run with 1 worker to establish the golden standard
	m1 := recursion.NewManager(
		client,
		excludeFilters,
		reader,
		strategy,
		maxDepth,
		recurseOnFilters,
		true, // normalizePaths
		true, // collapseSlashes
		nil, nil, nil, nil, 0, nil, nil,
	)

	ctx := context.Background()
	ch1 := m1.Run(ctx, seeds, 1)
	var w1Results []engine.Result
	for r := range ch1 {
		w1Results = append(w1Results, r)
	}
	sortResults(w1Results)

	// Run with configured workers matrix
	for _, wN := range lcfg.workerCounts {
		if wN == 1 {
			continue
		}
		t.Run(fmt.Sprintf("Workers-%d", wN), func(t *testing.T) {
			mN := recursion.NewManager(
				client,
				excludeFilters,
				reader,
				strategy,
				maxDepth,
				recurseOnFilters,
				true, // normalizePaths
				true, // collapseSlashes
				nil, nil, nil, nil, 0, nil, nil,
			)

			chN := mN.Run(ctx, seeds, wN)
			var wNResults []engine.Result
			for r := range chN {
				wNResults = append(wNResults, r)
			}
			sortResults(wNResults)

			// Compare result counts
			if len(w1Results) != len(wNResults) {
				reportDeterminismFailure(t, len(w1Results), len(wNResults), wN)
				return
			}

			// Compare individual result content & ordering
			for i := range w1Results {
				r1 := w1Results[i]
				rN := wNResults[i]

				if r1.URL != rN.URL || r1.StatusCode != rN.StatusCode || r1.Depth != rN.Depth {
					t.Errorf(`FAILED

Result content mismatch at sorted index %d

worker=1:
URL: %s, Status: %d, Depth: %d

worker=%d:
URL: %s, Status: %d, Depth: %d

status:
RELEASE BLOCKER`, i, r1.URL, r1.StatusCode, r1.Depth, wN, rN.URL, rN.StatusCode, rN.Depth)
					return
				}
			}

			// Check formatters output
			testFormatters(t, w1Results, wNResults, wN)
		})
	}

	return w1Results
}

func TestHardening_Static(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Static Root"))
	}))
	defer srv.Close()

	reader := testHardeningStaticReader{
		words: []string{"some", "path"},
	}

	results := runDeterminismTest(t, []string{srv.URL}, reader, 0, recursion.BFS, func(c *http.Client) {
		c.Transport = http.DefaultTransport
	})

	if len(results) != 1 {
		t.Errorf("Expected 1 result for static scan with maxdepth=0, got %d", len(results))
	}
}

func TestHardening_100(t *testing.T) {
	runHardeningSizeTest(t, 100)
}

func TestHardening_1000(t *testing.T) {
	runHardeningSizeTest(t, 1000)
}

func TestHardening_10000(t *testing.T) {
	runHardeningSizeTest(t, 10000)
}

func runHardeningSizeTest(t *testing.T, baseSize int) {
	level := getBenchmarkLevel()

	// Scale baseSize based on level
	wordlistSize := baseSize
	switch level {
	case 1:
		wordlistSize = 5
	case 2:
		wordlistSize = 20
	case 3:
		wordlistSize = 100
	case 4:
		wordlistSize = 500
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var words []string
	for i := 0; i < wordlistSize; i++ {
		words = append(words, fmt.Sprintf("word-%d", i))
	}
	reader := testHardeningStaticReader{words: words}

	results := runDeterminismTest(t, []string{srv.URL}, reader, 1, recursion.BFS, nil)
	expectedCount := 1 + wordlistSize
	if len(results) != expectedCount {
		t.Errorf("Expected %d results, got %d", expectedCount, len(results))
	}
}

func TestHardening_Circular(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reader := testHardeningStaticReader{
		words: []string{"a", "b", "a", "b"},
	}

	results := runDeterminismTest(t, []string{srv.URL}, reader, 3, recursion.BFS, nil)

	visited := make(map[string]struct{})
	for _, r := range results {
		if _, seen := visited[r.URL]; seen {
			t.Errorf("Duplicate URL discovered in circular run: %s", r.URL)
		}
		visited[r.URL] = struct{}{}
	}
}

func TestHardening_Deep(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reader := testHardeningStaticReader{
		words: []string{"depth"},
	}

	maxDepth := uint16(5)
	if getBenchmarkLevel() >= 3 {
		maxDepth = 15
	}

	results := runDeterminismTest(t, []string{srv.URL}, reader, maxDepth, recursion.BFS, nil)
	expected := int(maxDepth) + 1
	if len(results) != expected {
		t.Errorf("Expected %d results for deep scan, got %d", expected, len(results))
	}
}

func TestHardening_Breadth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	level := getBenchmarkLevel()
	breadthSize := 20
	if level >= 3 {
		breadthSize = 100
	}

	var words []string
	for i := 0; i < breadthSize; i++ {
		words = append(words, fmt.Sprintf("breadth-%d", i))
	}
	reader := testHardeningStaticReader{words: words}

	results := runDeterminismTest(t, []string{srv.URL}, reader, 1, recursion.BFS, nil)
	expected := 1 + breadthSize
	if len(results) != expected {
		t.Errorf("Expected %d results, got %d", expected, len(results))
	}
}

func TestHardening_MultiplePaths(t *testing.T) {
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv2.Close()

	reader := testHardeningStaticReader{
		words: []string{"some", "path"},
	}

	results := runDeterminismTest(t, []string{srv1.URL, srv2.URL}, reader, 1, recursion.BFS, nil)
	expected := 6
	if len(results) != expected {
		t.Errorf("Expected %d results for multi-path scan, got %d", expected, len(results))
	}
}

func TestHardening_MixedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "ok") {
			w.WriteHeader(http.StatusOK)
		} else if strings.Contains(path, "redir301") {
			w.WriteHeader(http.StatusMovedPermanently)
		} else if strings.Contains(path, "redir302") {
			w.WriteHeader(http.StatusFound)
		} else if strings.Contains(path, "forbidden") {
			w.WriteHeader(http.StatusForbidden)
		} else if strings.Contains(path, "notfound") {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	reader := testHardeningStaticReader{
		words: []string{"ok", "redir301", "redir302", "forbidden", "notfound"},
	}

	results := runDeterminismTest(t, []string{srv.URL}, reader, 2, recursion.BFS, nil)
	for _, r := range results {
		if strings.Contains(r.URL, "/notfound/") {
			t.Errorf("Error: recursed under path 'notfound' which has status 404: %s", r.URL)
		}
	}
}

func TestHardening_Duplicates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reader := testHardeningStaticReader{
		words: []string{"dup", "dup", "another", "another"},
	}

	seeds := []string{srv.URL, srv.URL, srv.URL + "/"}
	results := runDeterminismTest(t, seeds, reader, 2, recursion.BFS, nil)

	visited := make(map[string]struct{})
	for _, r := range results {
		if _, seen := visited[r.URL]; seen {
			t.Errorf("Duplicate discovery found: %s", r.URL)
		}
		visited[r.URL] = struct{}{}
	}
}

func TestHardening_Race(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reader := testHardeningStaticReader{
		words: []string{"race1", "race2", "race3"},
	}

	runDeterminismTest(t, []string{srv.URL}, reader, 2, recursion.BFS, nil)
}

func TestHardening_Cancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reader := testHardeningStaticReader{
		words: []string{"a", "b", "c", "d", "e", "f", "g"},
	}

	excludeFilters, _ := status.Parse("404")
	recurseOnFilters, _ := status.Parse("200")

	m := recursion.NewManager(
		http.DefaultClient,
		excludeFilters,
		reader,
		recursion.BFS,
		3,
		recurseOnFilters,
		true,
		true,
		nil, nil, nil, nil, 0, nil, nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultsChan := m.Run(ctx, []string{srv.URL}, 4)

	for range resultsChan {
		cancel()
	}
}

func TestHardening_Timeouts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "slow") {
			time.Sleep(50 * time.Millisecond)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reader := testHardeningStaticReader{
		words: []string{"slow", "fast"},
	}

	client := &http.Client{
		Timeout: 10 * time.Millisecond,
	}

	excludeFilters, _ := status.Parse("404")
	recurseOnFilters, _ := status.Parse("200")

	m := recursion.NewManager(
		client,
		excludeFilters,
		reader,
		recursion.BFS,
		1,
		recurseOnFilters,
		true,
		true,
		nil, nil, nil, nil, 0, nil, nil,
	)

	ch := m.Run(context.Background(), []string{srv.URL}, 4)
	for range ch {
	}
}

func TestHardening_Adaptive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nSitemap: /sitemap.xml\n"))
			return
		}
		if r.URL.Path == "/sitemap.xml" {
			_, _ = w.Write([]byte(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>/sitemap-path-1</loc></url>
<url><loc>/sitemap-path-2</loc></url>
</urlset>`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reader := testHardeningStaticReader{
		words: []string{"normal-word"},
	}

	results := runDeterminismTest(t, []string{srv.URL}, reader, 1, recursion.BFS, nil)

	foundSitemapPath := false
	for _, r := range results {
		if strings.Contains(r.URL, "sitemap-path-1") {
			foundSitemapPath = true
			break
		}
	}
	if !foundSitemapPath {
		t.Error("Expected sitemap paths to be crawled, but none found")
	}
}

func TestHardening_Profiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	profileNames := []string{"scan/base", "scan/maniac", "scan-extra/laravel", "scan-extra/wordpress"}
	store := profile.NewStore()

	reader := testHardeningStaticReader{
		words: []string{"path1", "path2"},
	}

	for _, name := range profileNames {
		t.Run(name, func(t *testing.T) {
			p, err := store.Load(name)
			if err != nil {
				t.Fatalf("Failed to load profile %s: %v", name, err)
			}

			var overlay scanProfile.Overlay
			if err := p.Decode(&overlay); err != nil {
				t.Fatalf("Failed to decode profile %s: %v", name, err)
			}

			threads := 32
			if overlay.Threads != nil {
				threads = int(*overlay.Threads)
			}

			cfg := config.Default()
			cfg.Recursive = true
			cfg.MaxDepth = 1

			excludeFilters, _ := status.Parse("404")
			recurseOnFilters, _ := status.Parse("200")

			m := recursion.NewManager(
				http.DefaultClient,
				excludeFilters,
				reader,
				recursion.BFS,
				cfg.MaxDepth,
				recurseOnFilters,
				true,
				true,
				nil, nil, nil, nil, 0, nil, nil,
			)

			level := getBenchmarkLevel()
			lcfg := getLevelConfig(level)
			maxWorkers := threads
			if maxWorkers > lcfg.workerCounts[len(lcfg.workerCounts)-1] {
				maxWorkers = lcfg.workerCounts[len(lcfg.workerCounts)-1]
			}

			ch := m.Run(context.Background(), []string{srv.URL}, maxWorkers)
			var results []engine.Result
			for r := range ch {
				results = append(results, r)
			}

			if len(results) != 3 {
				t.Errorf("Expected 3 results, got %d", len(results))
			}
		})
	}
}
