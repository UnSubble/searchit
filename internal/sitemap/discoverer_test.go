package sitemap_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/sitemap"
)

func TestDiscoverer_Discover(t *testing.T) {
	var (
		mu          sync.Mutex
		requests    []string
		discoveries []string
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

	disc.Discover(context.Background(), []string{srv.URL + "/sitemap.xml"}, func(path string) {
		mu.Lock()
		discoveries = append(discoveries, path)
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
	expectedDiscoveries := []string{"/relative-path", "/absolute-path?q=test"}
	if len(discoveries) != len(expectedDiscoveries) {
		t.Fatalf("expected discoveries: %v, got: %v", expectedDiscoveries, discoveries)
	}
	for i, path := range discoveries {
		if path != expectedDiscoveries[i] {
			t.Errorf("discovery[%d] expected %q, got %q", i, expectedDiscoveries[i], path)
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
