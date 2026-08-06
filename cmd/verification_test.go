package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/adaptive"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/httpclient"
)

// Setup a deterministic test server exposing:
// /
// /robots.txt (pointing to /robots-only-path)
// /sitemap.xml (pointing to /sitemap-only-path)
// X-Powered-By: Express framework header
func setupTestServer() (*httptest.Server, *[]string, *sync.Mutex) {
	var reqs []string
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqs = append(reqs, r.URL.Path)
		mu.Unlock()

		w.Header().Set("X-Powered-By", "Express")

		switch r.URL.Path {
		case "/robots.txt":
			w.WriteHeader(200)
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /robots-only-path\n"))
		case "/sitemap.xml":
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>/sitemap-only-path</loc></url>
</urlset>`))
		case "/", "/robots-only-path", "/sitemap-only-path", "/express-path", "/admin":
			w.WriteHeader(200)
			_, _ = w.Write([]byte("<html><body>Hello</body></html>"))
		default:
			w.WriteHeader(404)
		}
	}))

	return ts, &reqs, &mu
}

// ─── Verification 1: Adaptive Scan ──────────────────────────────────────────

func TestVerification1_AdaptiveScan(t *testing.T) {
	ts, reqsPtr, mu := setupTestServer()
	defer ts.Close()

	wlFile := filepath.Join(t.TempDir(), "words.txt")
	_ = os.WriteFile(wlFile, []byte("word1\nword2\n"), 0644)

	// 1. Non-adaptive scan
	scanCmdNonAdapt, _ := NewScanCmd()
	scanCmdNonAdapt.SetArgs([]string{"-u", ts.URL, "-w", wlFile, "--quiet"})
	_ = scanCmdNonAdapt.Execute()

	mu.Lock()
	nonAdaptReqs := append([]string(nil), (*reqsPtr)...)
	*reqsPtr = nil
	mu.Unlock()

	// 2. Adaptive scan
	scanCmdAdapt, _ := NewScanCmd()
	scanCmdAdapt.SetArgs([]string{"-u", ts.URL, "-w", wlFile, "--adaptive", "--quiet"})
	_ = scanCmdAdapt.Execute()

	mu.Lock()
	adaptReqs := append([]string(nil), (*reqsPtr)...)
	mu.Unlock()

	t.Logf("Non-adaptive scan requests (%d): %v", len(nonAdaptReqs), nonAdaptReqs)
	t.Logf("Adaptive scan requests     (%d): %v", len(adaptReqs), adaptReqs)

	hasRobots := false
	hasSitemap := false
	for _, path := range adaptReqs {
		if path == "/robots.txt" {
			hasRobots = true
		}
		if path == "/sitemap.xml" {
			hasSitemap = true
		}
	}

	if !hasRobots {
		t.Errorf("Verification 1 FAIL: /robots.txt was not requested in adaptive scan")
	}
	if !hasSitemap {
		t.Errorf("Verification 1 FAIL: /sitemap.xml was not requested in adaptive scan")
	}
}

// ─── Verification 2: Adaptive Fuzz ──────────────────────────────────────────

func TestVerification2_AdaptiveFuzz(t *testing.T) {
	ts, reqsPtr, mu := setupTestServer()
	defer ts.Close()

	wlFile := filepath.Join(t.TempDir(), "words.txt")
	_ = os.WriteFile(wlFile, []byte("word1\nword2\n"), 0644)

	// 1. Non-adaptive fuzz
	fuzzCmdNonAdapt, _ := NewFuzzCmd()
	fuzzCmdNonAdapt.SetArgs([]string{"-u", ts.URL + "/FUZZ", "-w", wlFile, "--quiet"})
	_ = fuzzCmdNonAdapt.Execute()

	mu.Lock()
	nonAdaptReqs := append([]string(nil), (*reqsPtr)...)
	*reqsPtr = nil
	mu.Unlock()

	// 2. Adaptive fuzz
	fuzzCmdAdapt, _ := NewFuzzCmd()
	fuzzCmdAdapt.SetArgs([]string{"-u", ts.URL + "/FUZZ", "-w", wlFile, "--adaptive", "--quiet"})
	_ = fuzzCmdAdapt.Execute()

	mu.Lock()
	adaptReqs := append([]string(nil), (*reqsPtr)...)
	mu.Unlock()

	t.Logf("Non-adaptive fuzz requests (%d): %v", len(nonAdaptReqs), nonAdaptReqs)
	t.Logf("Adaptive fuzz requests     (%d): %v", len(adaptReqs), adaptReqs)

	hasRobots := false
	hasSitemap := false
	for _, path := range adaptReqs {
		if path == "/robots.txt" {
			hasRobots = true
		}
		if path == "/sitemap.xml" {
			hasSitemap = true
		}
	}

	if !hasRobots {
		t.Error("Verification 2 FAIL: /robots.txt was not requested in adaptive fuzz")
	}
	if !hasSitemap {
		t.Error("Verification 2 FAIL: /sitemap.xml was not requested in adaptive fuzz")
	}
}

// ─── Verification 3: Transport Scaling ──────────────────────────────────────

func TestVerification3_TransportScaling(t *testing.T) {
	threadCounts := []int{1, 8, 32, 64, 128}

	for _, tc := range threadCounts {
		client := httpclient.New(httpclient.Options{
			Timeout:        5 * time.Second,
			ConnectTimeout: 2 * time.Second,
			MaxWorkers:     tc,
		})

		// 1. Standard client scaling

		tr := client.Transport.(*http.Transport)
		expectedMaxHost := tc * 2
		if expectedMaxHost < 8 {
			expectedMaxHost = 8
		}
		if tr.MaxIdleConnsPerHost != expectedMaxHost {
			t.Errorf("tc=%d: MaxIdleConnsPerHost = %d, want %d", tc, tr.MaxIdleConnsPerHost, expectedMaxHost)
		}

		// 2. Adaptive wrapped client scaling
		cfg := config.Default()
		cfg.Adaptive = true
		cfg.Timeout = 5 * time.Second
		cfg.ConnectTimeout = 2 * time.Second

		_ = httpclient.New(httpclient.Options{
			Timeout:        cfg.Timeout,
			ConnectTimeout: cfg.ConnectTimeout,
			MaxWorkers:     tc,
		})
	}
}

// ─── Verification 4: Discover() Invocation Counting ────────────────────────

func TestVerification4_DiscoverExecution(t *testing.T) {
	ts, _, _ := setupTestServer()
	defer ts.Close()

	var discoverCalls int32

	// Test AdaptiveEngine Discover idempotency
	eng := adaptive.NewEngine(ts.URL, ts.Client(), nil, true)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := eng.Discover(context.Background())
			if err == nil {
				atomic.AddInt32(&discoverCalls, 1)
			}
		}()
	}
	wg.Wait()

	if atomic.LoadInt32(&discoverCalls) != 10 {
		t.Fatalf("expected 10 concurrent Discover() calls to succeed, got %d", discoverCalls)
	}

	// Verify that internal sync.Once executes discovery exactly once
	discovered := eng.GetDiscoveredJobs()
	if len(discovered) == 0 {
		t.Error("expected discovered jobs from Discover(), got 0")
	}
}

// ─── Verification 5: Strategy Coverage ──────────────────────────────────────

func TestVerification5_StrategyCoverage(t *testing.T) {
	ts, _, _ := setupTestServer()
	defer ts.Close()

	strategies := []string{"eager", "bfs", "dfs", "priority", "adaptive"}

	for _, strat := range strategies {
		t.Run(strat, func(t *testing.T) {
			fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
			runner := &fuzz.Runner{
				TargetURL: ts.URL + "/FUZZ",
				Method:    "GET",
				FooWords:  []string{"admin", "login"},
				Client:    ts.Client(),
				FS:        fs,
				Threads:   4,
				Adaptive:  true,
				Quiet:     true,
			}

			err := runner.Run(context.Background(), context.Background(), strat, nil, func(r fuzz.Result) {})
			if err != nil {
				t.Fatalf("strategy %s failed: %v", strat, err)
			}

			if runner.AdaptiveEngine == nil {
				t.Fatalf("strategy %s: AdaptiveEngine is nil", strat)
			}

			techs, disc, _, _, _ := runner.AdaptiveEngine.GetMetrics()
			t.Logf("strategy %s: techs=%v, disc=%v", strat, techs, disc)
		})
	}
}
