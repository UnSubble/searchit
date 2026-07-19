package adaptive

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/robots"
	"github.com/unsubble/searchit/internal/sitemap"
)

// Collector encapsulates target header technology and robots.txt/sitemap.xml collection.
type Collector struct {
	TargetURL           string
	Client              *http.Client
	Cache               *fingerprint.Cache
	RobotsDirectives    []string
	SitemapPaths        []string
	RobotsDiscovered    bool
	SitemapDiscovered   bool
	PrioritizedSegments map[string]bool
	PrioritizedPaths    map[string]bool
	LaravelDetected     bool
	WPDetected          bool
	ExpressDetected     bool
}

// NewCollector instantiates a new signal collector.
func NewCollector(targetURL string, client *http.Client, cache *fingerprint.Cache) *Collector {
	return &Collector{
		TargetURL:           targetURL,
		Client:              client,
		Cache:               cache,
		PrioritizedSegments: make(map[string]bool),
		PrioritizedPaths:    make(map[string]bool),
	}
}

// Execute performs target signal collection.
func (c *Collector) Execute(ctx context.Context) error {
	u, err := url.Parse(c.TargetURL)
	if err != nil {
		return err
	}
	hostRoot := fmt.Sprintf("%s://%s", u.Scheme, u.Host)

	robotsBody, _, err := robots.Download(ctx, c.Client, hostRoot)
	if err == nil {
		if directives, err := robots.Parse(robotsBody); err == nil {
			c.RobotsDiscovered = true
			for _, dir := range directives {
				if dir.Path != "" {
					c.RobotsDirectives = append(c.RobotsDirectives, dir.Path)
				}
			}
		}
		robotsBody.Close()
	}

	disc, err := sitemap.NewDiscoverer(c.Client, c.Cache, hostRoot)
	if err == nil {
		disc.Discover(ctx, []string{hostRoot + "/sitemap.xml"}, func(path string, origin string) {
			c.SitemapDiscovered = true
			c.SitemapPaths = append(c.SitemapPaths, path)
		})
	}

	// Tech detection
	if c.Cache != nil {
		if fp := c.Cache.Get(u.Host); fp != nil {
			for _, sig := range fp.Signals() {
				val := strings.ToLower(sig.Value)
				src := strings.ToLower(sig.Source)
				if strings.Contains(val, "laravel") {
					c.LaravelDetected = true
				}
				if strings.Contains(val, "wordpress") || strings.Contains(src, "wordpress") {
					c.WPDetected = true
				}
				if strings.Contains(val, "express") {
					c.ExpressDetected = true
				}
			}
		}
	}

	// Build lookup maps
	for _, p := range c.RobotsDirectives {
		c.PrioritizedPaths[strings.Trim(p, "/")] = true
		parts := strings.Split(p, "/")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				c.PrioritizedSegments[strings.ToLower(part)] = true
			}
		}
	}

	for _, p := range c.SitemapPaths {
		c.PrioritizedPaths[strings.Trim(p, "/")] = true
		parts := strings.Split(p, "/")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				c.PrioritizedSegments[strings.ToLower(part)] = true
			}
		}
	}

	return nil
}
