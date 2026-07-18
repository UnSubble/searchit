package cmd

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/httpclient"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/wordlist"
)

func getBenchmarkLevel() int {
	levelStr := os.Getenv("BENCHMARK_LEVEL")
	if levelStr == "" {
		return 1
	}
	var level int
	if _, err := fmt.Sscan(levelStr, &level); err != nil {
		return 1
	}
	return level
}

func TestChaos_Scans(t *testing.T) {
	// 1. Setup a chaotic mock server that generates loops, redirects, delays, and random responses
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/loop-a":
			w.Header().Set("Location", "/loop-b")
			w.WriteHeader(http.StatusMovedPermanently)
		case p == "/loop-b":
			w.Header().Set("Location", "/loop-a")
			w.WriteHeader(http.StatusFound)
		case p == "/self-loop":
			w.Header().Set("Location", "/self-loop")
			w.WriteHeader(http.StatusTemporaryRedirect)
		case p == "/heavy-delay":
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		case p == "/chain-1":
			w.Header().Set("Location", "/chain-2")
			w.WriteHeader(http.StatusMovedPermanently)
		case p == "/chain-2":
			w.Header().Set("Location", "/chain-3")
			w.WriteHeader(http.StatusFound)
		case p == "/chain-3":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "chaos_wl.txt")
	if err := os.WriteFile(wlPath, []byte("loop-a\nloop-b\nself-loop\nheavy-delay\nchain-1\n"), 0600); err != nil {
		t.Fatalf("failed to write chaos wordlist: %v", err)
	}

	// 2. Perform chaos runs under context cancellations and timeouts
	t.Run("concurrent cancellation and timeout storm", func(t *testing.T) {
		wl := wordlist.FileReader{Path: wlPath}

		// Run manager multiple times under cancellation pressure
		for i := 0; i < 50; i++ {
			runCtx, runCancel := context.WithCancel(context.Background())

			// Start a canceler goroutine for this specific run
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				time.Sleep(time.Duration(rand.Intn(10)+1) * time.Millisecond)
				runCancel()
			}()

			cfg := config.Default()
			cfg.URLs = []string{srv.URL}
			cfg.Recursive = true
			cfg.MaxDepth = 3
			cfg.Threads = 64
			cfg.FollowRedirects = true
			cfg.MaxRedirects = 10

			client := srv.Client()
			manager := recursion.NewManager(
				client,
				status.MustParse("404"),
				wl,
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

			results := manager.Run(runCtx, cfg.URLs, cfg.Threads)
			for range results {
				// Consume results to prevent blocking
			}
			wg.Wait()
		}
	})
}

func TestMonkey_RandomScans(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Technology detection signatures
		if rand.Float32() < 0.2 {
			w.Header().Set("X-Powered-By", "PHP/8.1")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>Laravel static page</html>"))
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "monkey_wl.txt")
	wlWords := []string{"admin", "login", "api", "robots.txt", "sitemap.xml"}
	if err := os.WriteFile(wlPath, []byte("admin\nlogin\napi\nrobots.txt\nsitemap.xml\n"), 0600); err != nil {
		t.Fatalf("failed to write monkey wordlist: %v", err)
	}

	level := getBenchmarkLevel()
	var runs int
	switch level {
	case 1:
		runs = 20
	case 2:
		runs = 100
	case 3:
		runs = 500
	case 4:
		runs = 2000
	default:
		runs = 5000
	}

	t.Logf("Running %d monkey iterations (BENCHMARK_LEVEL=%d)", runs, level)

	rng := rand.New(rand.NewSource(12345))

	for i := 0; i < runs; i++ {
		// Randomize config values
		workers := rng.Intn(128) + 1
		recursive := rng.Float32() < 0.5
		followRedirects := rng.Float32() < 0.5
		maxRedirects := rng.Intn(15) + 1
		maxDepth := uint16(rng.Intn(3) + 1)
		delayMs := rng.Intn(5)

		wl := wordlist.FileReader{Path: wlPath}

		// Create a separate test run context to catch panics/races
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Monkey run %d panicked: %v", i, r)
				}
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			client := httpclient.NewWithMaxRedirects(10*time.Second, 10*time.Second, followRedirects, maxRedirects, "")
			fs, _ := filter.NewFilterSuite("", "404", "", "", nil, nil, nil, nil)

			if recursive {
				manager := recursion.NewManager(
					client,
					status.MustParse("404"),
					wl,
					recursion.BFS,
					maxDepth,
					status.MustParse("200,301,302,403"),
					false,
					false,
					nil,
					nil,
					nil,
					nil,
					time.Duration(delayMs)*time.Millisecond,
					nil,
					fingerprint.NewCache(),
				)
				results := manager.Run(ctx, []string{srv.URL}, workers)
				for range results {
				}
			} else {
				// Fuzzing loop
				jobs := make(chan fuzz.Job, workers)
				go func() {
					defer close(jobs)
					for _, word := range wlWords {
						jobs <- fuzz.Job{
							URL:    srv.URL + "/" + word,
							Method: "GET",
						}
					}
				}()

				collector := stats.NewCollector()
				results := fuzz.Start(
					ctx,
					client,
					fs,
					workers,
					time.Duration(delayMs)*time.Millisecond,
					nil,
					jobs,
					collector,
				)
				for range results {
				}
			}
		}()
	}
}
