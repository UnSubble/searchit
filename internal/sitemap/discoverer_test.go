package sitemap_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/sitemap"
)

func gzipCompress(data []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(data)
	_ = w.Close()
	return buf.Bytes()
}

func TestDiscoverer_Discover(t *testing.T) {
	var (
		mu          sync.Mutex
		requests    []string
		discoveries []struct {
			path   string
			origin string
		}
	)

	// Mock server that serves nested sitemaps and target files
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.Path)
		mu.Unlock()

		switch r.URL.Path {
		case "/sitemap.xml":
			// A sitemap index pointing to sub-sitemaps
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <sitemap>
      <loc>/sitemap1.xml</loc>
   </sitemap>
   <sitemap>
      <!-- Foreign host index entry should be ignored during recursion download -->
      <loc>http://foreign.com/sitemap_foreign.xml</loc>
   </sitemap>
   <sitemap>
      <!-- Duplicate reference to verify loop prevention -->
      <loc>/sitemap.xml</loc>
   </sitemap>
</sitemapindex>`))

		case "/sitemap1.xml":
			// Sub-sitemap with standard and relative URLs, foreign URLs, query parameters, and fragments
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <url>
      <loc>/relative-path</loc>
      <lastmod>2026-07-13</lastmod>
   </url>
   <url>
      <!-- Absolute path same host -->
      <loc>` + srv.URL + `/absolute-path?q=test#fragment</loc>
      <changefreq>always</changefreq>
      <priority>1.0</priority>
   </url>
   <url>
      <!-- Foreign host path should be ignored -->
      <loc>http://foreign.com/bad-path</loc>
   </url>
</urlset>`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	fpCache := fingerprint.NewCache()
	disc, err := sitemap.NewDiscoverer(http.DefaultClient, fpCache, srv.URL)
	if err != nil {
		t.Fatalf("failed to create discoverer: %v", err)
	}

	disc.Discover(context.Background(), []string{srv.URL + "/sitemap.xml"}, func(path string, origin string) {
		mu.Lock()
		discoveries = append(discoveries, struct {
			path   string
			origin string
		}{path: path, origin: origin})
		mu.Unlock()
	})

	// Verify request paths
	expectedRequests := []string{"/sitemap.xml", "/sitemap1.xml"}
	if len(requests) != len(expectedRequests) {
		t.Fatalf("expected requests: %v, got: %v", expectedRequests, requests)
	}
	for i, path := range requests {
		if path != expectedRequests[i] {
			t.Errorf("request[%d] expected %q, got %q", i, expectedRequests[i], path)
		}
	}

	// Verify discoveries (only local paths, resolved relative links, ignored fragments, preserved queries)
	expectedDiscoveries := []struct {
		path   string
		origin string
	}{
		{path: "/relative-path", origin: engine.OriginSitemapIdx},
		{path: "/absolute-path?q=test", origin: engine.OriginSitemapIdx},
	}
	if len(discoveries) != len(expectedDiscoveries) {
		t.Fatalf("expected discoveries: %+v, got: %+v", expectedDiscoveries, discoveries)
	}
	for i, item := range discoveries {
		if item.path != expectedDiscoveries[i].path {
			t.Errorf("discovery[%d] expected path %q, got %q", i, expectedDiscoveries[i].path, item.path)
		}
		if item.origin != expectedDiscoveries[i].origin {
			t.Errorf("discovery[%d] expected origin %q, got %q", i, expectedDiscoveries[i].origin, item.origin)
		}
	}

	// Verify target host fingerprint observations
	u, _ := url.Parse(srv.URL)
	fp := fpCache.Get(u.Host)
	if fp == nil {
		t.Fatal("expected target fingerprint to be created, but got nil")
	}

	signals := fp.Signals()
	var (
		hasIndexSig      bool
		hasURLSig        bool
		hasLastmodSig    bool
		hasChangefreqSig bool
		hasPrioritySig   bool
	)

	for _, s := range signals {
		switch s.Source {
		case "sitemap:index":
			if strings.HasSuffix(s.Value, "/sitemap1.xml") {
				hasIndexSig = true
			}
		case "sitemap:url":
			if strings.Contains(s.Value, "/relative-path") {
				hasURLSig = true
			}
		case "sitemap:lastmod":
			if strings.Contains(s.Value, "/relative-path|2026-07-13") {
				hasLastmodSig = true
			}
		case "sitemap:changefreq":
			if strings.Contains(s.Value, "/absolute-path?q=test|always") {
				hasChangefreqSig = true
			}
		case "sitemap:priority":
			if strings.Contains(s.Value, "/absolute-path?q=test|1.0") {
				hasPrioritySig = true
			}
		}
	}

	if !hasIndexSig {
		t.Error("expected fingerprint to contain sitemap:index signal")
	}
	if !hasURLSig {
		t.Error("expected fingerprint to contain sitemap:url signal")
	}
	if !hasLastmodSig {
		t.Error("expected fingerprint to contain sitemap:lastmod signal")
	}
	if !hasChangefreqSig {
		t.Error("expected fingerprint to contain sitemap:changefreq signal")
	}
	if !hasPrioritySig {
		t.Error("expected fingerprint to contain sitemap:priority signal")
	}
}

func TestDiscoverer_Compressed(t *testing.T) {
	var (
		mu          sync.Mutex
		discoveries []struct {
			path   string
			origin string
		}
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml.gz":
			w.Header().Set("Content-Type", "application/x-gzip")
			w.WriteHeader(http.StatusOK)
			payload := gzipCompress([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <url><loc>/gzipped-path</loc></url>
</urlset>`))
			_, _ = w.Write(payload)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	disc, err := sitemap.NewDiscoverer(http.DefaultClient, nil, srv.URL)
	if err != nil {
		t.Fatalf("failed to create discoverer: %v", err)
	}

	disc.Discover(context.Background(), []string{srv.URL + "/sitemap.xml.gz"}, func(path string, origin string) {
		mu.Lock()
		discoveries = append(discoveries, struct {
			path   string
			origin string
		}{path: path, origin: origin})
		mu.Unlock()
	})

	if len(discoveries) != 1 || discoveries[0].path != "/gzipped-path" || discoveries[0].origin != engine.OriginSitemapXml {
		t.Errorf("expected 1 gzipped discovery, got: %+v", discoveries)
	}
}

func TestDiscoverer_EdgeCases(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []string
		yielded  []struct {
			path   string
			origin string
		}
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.Path)
		mu.Unlock()

		switch r.URL.Path {
		case "/loop1.xml":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <sitemap><loc>/loop2.xml</loc></sitemap>
</sitemapindex>`))
		case "/loop2.xml":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <sitemap><loc>/loop1.xml</loc></sitemap>
</sitemapindex>`))
		case "/duplicates.xml":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <url><loc>/dup</loc></url>
   <url><loc>/dup</loc></url>
</urlset>`))
		case "/error500.xml":
			w.WriteHeader(http.StatusInternalServerError)
		case "/empty.xml":
			w.WriteHeader(http.StatusOK)
		case "/malformed.xml":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <url><loc>/partial-success</loc></url>
   <url><loc>/broken-loc`)) // missing closing tag/malformed
		case "/utf8.xml":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <url><loc>/%d1%82%d0%b5%d1%81%d1%82</loc></url>
</urlset>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	disc, err := sitemap.NewDiscoverer(http.DefaultClient, nil, srv.URL)
	if err != nil {
		t.Fatalf("failed to create discoverer: %v", err)
	}

	yield := func(path string, origin string) {
		mu.Lock()
		yielded = append(yielded, struct {
			path   string
			origin string
		}{path: path, origin: origin})
		mu.Unlock()
	}

	// 1. Verify loop prevention / cycles terminate gracefully
	disc.Discover(context.Background(), []string{srv.URL + "/loop1.xml"}, yield)
	mu.Lock()
	reqs := make([]string, len(requests))
	copy(reqs, requests)
	requests = nil
	mu.Unlock()

	if len(reqs) != 2 {
		t.Errorf("expected 2 cyclic sitemap requests, got %d: %v", len(reqs), reqs)
	}

	// 2. Verify duplicate URL items are yielded (deduplication is done at frontier integration/visited set level)
	disc.Discover(context.Background(), []string{srv.URL + "/duplicates.xml"}, yield)
	mu.Lock()
	dupsYielded := make([]struct {
		path   string
		origin string
	}, len(yielded))
	copy(dupsYielded, yielded)
	yielded = nil
	mu.Unlock()

	if len(dupsYielded) != 2 || dupsYielded[0].path != "/dup" || dupsYielded[1].path != "/dup" {
		t.Errorf("expected duplicate items to be yielded, got %v", dupsYielded)
	}

	// 3. Verify server error status codes (e.g. 500) and empty sitemaps handle cleanly
	disc.Discover(context.Background(), []string{
		srv.URL + "/error500.xml",
		srv.URL + "/empty.xml",
		srv.URL + "/nonexistent.xml",
	}, yield)

	mu.Lock()
	errorYielded := len(yielded)
	mu.Unlock()
	if errorYielded != 0 {
		t.Errorf("expected 0 items yielded from error/empty sitemaps, got %d", errorYielded)
	}

	// 4. Verify partial success with malformed XML
	disc.Discover(context.Background(), []string{srv.URL + "/malformed.xml"}, yield)
	mu.Lock()
	malformedYielded := make([]struct {
		path   string
		origin string
	}, len(yielded))
	copy(malformedYielded, yielded)
	yielded = nil
	mu.Unlock()

	if len(malformedYielded) != 1 || malformedYielded[0].path != "/partial-success" {
		t.Errorf("expected partial discovery success before malformed tag, got: %+v", malformedYielded)
	}

	// 5. Verify UTF-8 URLs
	disc.Discover(context.Background(), []string{srv.URL + "/utf8.xml"}, yield)
	mu.Lock()
	utf8Yielded := make([]struct {
		path   string
		origin string
	}, len(yielded))
	copy(utf8Yielded, yielded)
	yielded = nil
	mu.Unlock()

	if len(utf8Yielded) != 1 || utf8Yielded[0].path != "/тест" {
		t.Errorf("expected UTF-8 path, got: %q", utf8Yielded[0].path)
	}
}

func TestDiscoverer_Cancellation(t *testing.T) {
	var (
		mu      sync.Mutex
		yielded []string
	)

	// Slow server to guarantee we can cancel context mid-request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <url><loc>/path-after-cancellation</loc></url>
</urlset>`))
	}))
	t.Cleanup(srv.Close)

	disc, err := sitemap.NewDiscoverer(http.DefaultClient, nil, srv.URL)
	if err != nil {
		t.Fatalf("failed to create discoverer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	disc.Discover(ctx, []string{srv.URL + "/sitemap.xml"}, func(path string, origin string) {
		mu.Lock()
		yielded = append(yielded, path)
		mu.Unlock()
	})

	if len(yielded) != 0 {
		t.Errorf("expected 0 discoveries on context cancellation, got: %v", yielded)
	}
}

func TestDiscoverer_Concurrency(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <url><loc>/path</loc></url>
</urlset>`))
	}))
	t.Cleanup(srv.Close)

	disc, err := sitemap.NewDiscoverer(http.DefaultClient, nil, srv.URL)
	if err != nil {
		t.Fatalf("failed to create discoverer: %v", err)
	}

	const workers = 20
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(wID int) {
			defer wg.Done()
			disc.Discover(context.Background(), []string{srv.URL + fmt.Sprintf("/sitemap-%d.xml", wID)}, func(path string, origin string) {})
		}(i)
	}

	wg.Wait()
}
