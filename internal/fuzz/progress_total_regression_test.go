package fuzz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/wordlist"
)

func TestProgressTotal_EstimateCandidates_Invariants(t *testing.T) {
	runner := &Runner{
		TargetURL: "http://example.com/FUZZ",
		Method:    "GET",
	}

	// 1. EstimateCandidates with positive primary wordlist size
	total := runner.EstimateCandidates(100)
	if total != 100 {
		t.Errorf("expected EstimateCandidates(100) = 100, got %d", total)
	}

	// 2. EstimateCandidates with primary size 0 and empty FuzzWords should return 0 (not 1)
	totalZero := runner.EstimateCandidates(0)
	if totalZero != 0 {
		t.Errorf("expected EstimateCandidates(0) = 0 for FUZZ template, got %d", totalZero)
	}

	// 3. EstimateCandidates with FuzzWords fallback
	runner.FuzzWords = []string{"a", "b", "c", "d", "e"}
	totalFallback := runner.EstimateCandidates(0)
	if totalFallback != 5 {
		t.Errorf("expected EstimateCandidates(0) with len(FuzzWords)=5 to return 5, got %d", totalFallback)
	}
}

func TestProgressTotal_Lifecycle_Scenarios(t *testing.T) {
	// Setup test HTTP server with optional robots.txt / sitemap.xml
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("User-agent: *\nDisallow: /admin\n"))
		case "/sitemap.xml":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<xml></xml>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	words := []string{"admin", "login", "user", "api", "test", "v1", "v2", "dev", "staging", "prod"}
	wordCount := len(words)

	tests := []struct {
		name              string
		adaptive          bool
		strategy          string
		withRobotsSitemap bool
	}{
		{
			name:              "normal_fuzz_eager",
			adaptive:          false,
			strategy:          "eager",
			withRobotsSitemap: false,
		},
		{
			name:              "adaptive_fuzz_with_discoveries",
			adaptive:          true,
			strategy:          "adaptive",
			withRobotsSitemap: true,
		},
		{
			name:              "adaptive_fuzz_without_discoveries",
			adaptive:          true,
			strategy:          "adaptive",
			withRobotsSitemap: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			collector := stats.NewCollector()
			collector.SetIsFinite(true)
			collector.SetTotalCandidates(int64(wordCount))

			fs, err := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
			if err != nil {
				t.Fatalf("failed to create filter suite: %v", err)
			}

			client := srv.Client()
			if !tt.withRobotsSitemap {
				// Server returning 404 for discovery endpoints
				client = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				})).Client()
			}

			runner := &Runner{
				TargetURL: srv.URL + "/FUZZ",
				Method:    "GET",
				Client:    client,
				FS:        fs,
				Threads:   2,
				Collector: collector,
				Adaptive:  tt.adaptive,
			}

			primaryChan := make(chan string, len(words))
			for _, w := range words {
				primaryChan <- w
			}
			close(primaryChan)

			// Verify total before Run
			initSnap := collector.Snapshot()
			if initSnap.TotalWork != int64(wordCount) {
				t.Errorf("expected initial TotalWork = %d, got %d", wordCount, initSnap.TotalWork)
			}

			err = runner.Run(ctx, ctx, tt.strategy, primaryChan, func(r Result) {})
			if err != nil {
				t.Fatalf("Runner.Run failed: %v", err)
			}

			finalSnap := collector.Snapshot()
			completed := finalSnap.Tried + finalSnap.Skipped

			// Verification: Total work must never become 1 for multi-candidate scan
			if finalSnap.TotalWork == 1 && wordCount > 1 {
				t.Errorf("FAIL: TotalWork collapsed to 1 during scan! (got TotalWork=%d, wordCount=%d)", finalSnap.TotalWork, wordCount)
			}

			// Invariant: completed <= TotalWork when TotalWork > 0
			if finalSnap.TotalWork > 0 && completed > finalSnap.TotalWork {
				t.Errorf("FAIL: completed (%d) > TotalWork (%d)", completed, finalSnap.TotalWork)
			}

			// Invariant: 0 <= Progress <= 100.0%
			if finalSnap.TotalWork > 0 {
				p := float64(completed) / float64(finalSnap.TotalWork) * 100.0
				if p < 0.0 || p > 100.0 {
					t.Errorf("FAIL: Progress percentage out of bounds: %.2f%%", p)
				}
			}
		})
	}
}

func TestProgressTotal_CustomAndEmbeddedWordlist(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Custom File Wordlist
	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "custom.txt")
	fileContent := "path1\npath2\npath3\npath4\npath5\n"
	_ = os.WriteFile(wlPath, []byte(fileContent), 0644)

	fileReader := wordlist.FileReader{Path: wlPath}
	fileChan := make(chan string, 10)
	go func() {
		defer close(fileChan)
		_ = fileReader.Read(ctx, fileChan)
	}()

	var customWords []string
	for w := range fileChan {
		customWords = append(customWords, w)
	}

	if len(customWords) != 5 {
		t.Fatalf("expected 5 words read from file, got %d", len(customWords))
	}

	// 2. Embedded Wordlist
	embReader := wordlist.EmbeddedReader{}
	embCount, err := embReader.Count()
	if err != nil || embCount <= 0 {
		t.Fatalf("failed to count embedded wordlist: %v", err)
	}

	runnerEmb := &Runner{
		TargetURL: "http://localhost/FUZZ",
		Method:    "GET",
	}
	totalEmb := runnerEmb.EstimateCandidates(embCount)
	if totalEmb != int64(embCount) {
		t.Errorf("expected embedded EstimateCandidates = %d, got %d", embCount, totalEmb)
	}
}
