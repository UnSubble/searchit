package sitemap

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/unsubble/searchit/internal/fingerprint"
)

// Discoverer recursively fetches sitemaps and indexes, validates host ownership,
// records signals to the fingerprint engine, and yields discovered paths.
type Discoverer struct {
	client           *http.Client
	fingerprintCache *fingerprint.Cache
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
// the yield callback for each discovered path.
func (d *Discoverer) Discover(ctx context.Context, startURLs []string, yield func(path string)) {
	for _, rawURL := range startURLs {
		d.processSitemap(ctx, rawURL, yield)
	}
}

func (d *Discoverer) processSitemap(ctx context.Context, sitemapURLStr string, yield func(path string)) {
	normalizedSitemap := strings.TrimRight(strings.ToLower(sitemapURLStr), "/")
	if _, seen := d.visitedSitemaps[normalizedSitemap]; seen {
		return
	}
	d.visitedSitemaps[normalizedSitemap] = struct{}{}

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

	var fp *fingerprint.Fingerprint
	if d.fingerprintCache != nil {
		fp = d.fingerprintCache.GetOrCreate(d.targetURL.Host)
	}

	err = ParseStream(resp.Body, func(item XMLItem) {
		itemLocURL, err := url.Parse(item.Loc)
		if err != nil {
			return
		}
		resolvedURL := parentURL.ResolveReference(itemLocURL)
		resolvedURL.Fragment = ""
		resolvedURL.RawFragment = ""

		// Verify host matches the target URL's host exactly
		if resolvedURL.Host != d.targetURL.Host {
			return
		}

		// Log raw sitemap observations into the host fingerprint
		if fp != nil {
			if item.IsSitemap {
				fp.AddSignal(fingerprint.Signal{
					Source:     "sitemap:index",
					Value:      resolvedURL.String(),
					Confidence: fingerprint.Confidence(1.0),
				})
			} else {
				fp.AddSignal(fingerprint.Signal{
					Source:     "sitemap:url",
					Value:      resolvedURL.String(),
					Confidence: fingerprint.Confidence(1.0),
				})
				if item.LastMod != "" {
					fp.AddSignal(fingerprint.Signal{
						Source:     "sitemap:lastmod",
						Value:      resolvedURL.String() + "|" + item.LastMod,
						Confidence: fingerprint.Confidence(1.0),
					})
				}
				if item.ChangeFreq != "" {
					fp.AddSignal(fingerprint.Signal{
						Source:     "sitemap:changefreq",
						Value:      resolvedURL.String() + "|" + item.ChangeFreq,
						Confidence: fingerprint.Confidence(1.0),
					})
				}
				if item.Priority != "" {
					fp.AddSignal(fingerprint.Signal{
						Source:     "sitemap:priority",
						Value:      resolvedURL.String() + "|" + item.Priority,
						Confidence: fingerprint.Confidence(1.0),
					})
				}
			}
		}

		if item.IsSitemap {
			// Nested sitemap index: recurse
			d.processSitemap(ctx, resolvedURL.String(), yield)
		} else {
			// Ignore fragments
			resolvedURL.Fragment = ""
			resolvedURL.RawFragment = ""

			// Format path component (retaining query parameters if explicitly present)
			candidate := resolvedURL.Path
			if candidate == "" {
				candidate = "/"
			}
			if resolvedURL.RawQuery != "" {
				candidate += "?" + resolvedURL.RawQuery
			}
			yield(candidate)
		}
	})

	_ = err // tolerant of parsing errors
}
