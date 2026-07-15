package sitemap

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/fingerprint"
)

// Discoverer recursively fetches sitemaps and indexes, validates host ownership,
// records signals to the fingerprint engine, and yields discovered paths.
type Discoverer struct {
	client           *http.Client
	fingerprintCache *fingerprint.Cache
	mu               sync.Mutex // Protects visitedSitemaps
	visitedSitemaps  map[string]struct{}
	targetURL        *url.URL
}

// NewDiscoverer creates a Discoverer targeting the given base URL.
func NewDiscoverer(client *http.Client, fpCache *fingerprint.Cache, targetURLStr string) (*Discoverer, error) {
	u, err := url.Parse(targetURLStr)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}
	return &Discoverer{
		client:           client,
		fingerprintCache: fpCache,
		visitedSitemaps:  make(map[string]struct{}),
		targetURL:        u,
	}, nil
}

// Discover fetches and recursively processes the starting sitemap URLs.
// It normalizes and verifies host match for each candidate path, calling
// the yield callback for each discovered path with its origin metadata.
func (d *Discoverer) Discover(ctx context.Context, startURLs []string, yield func(path string, origin string)) {
	for _, rawURL := range startURLs {
		d.processSitemap(ctx, rawURL, false, yield)
	}
}

func (d *Discoverer) processSitemap(ctx context.Context, sitemapURLStr string, isIndex bool, yield func(path string, origin string)) {
	// Normalize the sitemap URL for visited set to prevent duplicate crawls and infinite loops
	normalizedSitemap := d.normalizeSitemapURL(sitemapURLStr)
	d.mu.Lock()
	if _, seen := d.visitedSitemaps[normalizedSitemap]; seen {
		d.mu.Unlock()
		return
	}
	d.visitedSitemaps[normalizedSitemap] = struct{}{}
	d.mu.Unlock()

	parentURL, err := url.Parse(sitemapURLStr)
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURLStr, nil)
	if err != nil {
		return
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	// Read magic bytes to transparently decompress gzipped sitemaps
	bufReader := bufio.NewReader(resp.Body)
	magic, err := bufReader.Peek(2)

	var streamReader io.Reader = bufReader
	if err == nil && len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gzReader, err := gzip.NewReader(bufReader)
		if err != nil {
			return
		}
		defer gzReader.Close()
		streamReader = gzReader
	}

	var fp *fingerprint.Fingerprint
	if d.fingerprintCache != nil {
		fp = d.fingerprintCache.GetOrCreate(d.targetURL.Host)
	}

	err = ParseStream(streamReader, func(item XMLItem) {
		itemLocURL, err := url.Parse(item.Loc)
		if err != nil {
			return
		}
		resolvedURL := parentURL.ResolveReference(itemLocURL)
		resolvedURL.Fragment = ""
		resolvedURL.RawFragment = ""

		// Verify host matches the target URL's host exactly to prevent SSRF
		if resolvedURL.Host != d.targetURL.Host {
			return
		}

		// Log raw sitemap observations into the host fingerprint
		if fp != nil {
			if item.IsSitemap {
				fp.AddSignal(fingerprint.Signal{
					Source: "sitemap:index",
					Value:  resolvedURL.String(),
				})
			} else {
				fp.AddSignal(fingerprint.Signal{
					Source: "sitemap:url",
					Value:  resolvedURL.String(),
				})
				if item.LastMod != "" {
					fp.AddSignal(fingerprint.Signal{
						Source: "sitemap:lastmod",
						Value:  resolvedURL.String() + "|" + item.LastMod,
					})
				}
				if item.ChangeFreq != "" {
					fp.AddSignal(fingerprint.Signal{
						Source: "sitemap:changefreq",
						Value:  resolvedURL.String() + "|" + item.ChangeFreq,
					})
				}
				if item.Priority != "" {
					fp.AddSignal(fingerprint.Signal{
						Source: "sitemap:priority",
						Value:  resolvedURL.String() + "|" + item.Priority,
					})
				}
			}
		}

		if item.IsSitemap {
			// Nested sitemap index: recurse
			d.processSitemap(ctx, resolvedURL.String(), true, yield)
		} else {
			// Format path component (retaining query parameters if explicitly present)
			candidate := resolvedURL.Path
			if candidate == "" {
				candidate = "/"
			}
			if resolvedURL.RawQuery != "" {
				candidate += "?" + resolvedURL.RawQuery
			}

			origin := engine.OriginSitemapXml
			if isIndex {
				origin = engine.OriginSitemapIdx
			}
			yield(candidate, origin)
		}
	})

	_ = err // tolerant of parsing errors
}

func (d *Discoverer) normalizeSitemapURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return strings.TrimRight(strings.ToLower(rawURL), "/")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.RawFragment = ""
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String()
}
