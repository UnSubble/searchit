package adaptive

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/robots"
	"github.com/unsubble/searchit/internal/sitemap"
	"github.com/unsubble/searchit/internal/wordlist"
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
	DiscoveredJobs      []engine.Job
	seenURLs            map[string]bool
	injectedFrameworks  map[string]bool
}

// NewCollector instantiates a new signal collector.
func NewCollector(targetURL string, client *http.Client, cache *fingerprint.Cache) *Collector {
	return &Collector{
		TargetURL:           targetURL,
		Client:              client,
		Cache:               cache,
		PrioritizedSegments: make(map[string]bool),
		PrioritizedPaths:    make(map[string]bool),
		seenURLs:            make(map[string]bool),
		injectedFrameworks:  make(map[string]bool),
	}
}

// Execute performs target signal collection.
func (c *Collector) Execute(ctx context.Context) error {
	u, err := url.Parse(c.TargetURL)
	if err != nil {
		return err
	}
	hostRoot := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	hostRootURL, _ := url.Parse(hostRoot)

	var robotsSitemaps []string

	// 1. Download & Parse robots.txt
	robotsBody, _, err := robots.Download(ctx, c.Client, hostRoot)
	if err == nil {
		directives, parseErr := robots.Parse(robotsBody)
		if parseErr == nil {
			c.RobotsDiscovered = true
			for _, dir := range directives {
				if dir.Type == robots.Sitemap {
					sitemapURL, sErr := url.Parse(dir.Path)
					if sErr == nil {
						resolvedSitemap := hostRootURL.ResolveReference(sitemapURL).String()
						robotsSitemaps = append(robotsSitemaps, resolvedSitemap)
					}
					continue
				}

				if dir.Path != "" {
					c.RobotsDirectives = append(c.RobotsDirectives, dir.Path)

					// Add signal to fingerprint if cache present
					if c.Cache != nil {
						fp := c.Cache.GetOrCreate(u.Host)
						source := "robots:allow"
						if dir.Type == robots.Disallow {
							source = "robots:disallow"
						}
						fp.AddSignal(fingerprint.Signal{
							Source: source,
							Value:  dir.Path,
						})
					}

					pathVal := strings.TrimSpace(dir.Path)
					if idx := strings.IndexAny(pathVal, "*$"); idx != -1 {
						pathVal = pathVal[:idx]
					}
					pathVal = strings.TrimSpace(pathVal)
					if pathVal != "" {
						if !strings.HasPrefix(pathVal, "/") {
							pathVal = "/" + pathVal
						}

						childURL, jErr := wordlist.Join(hostRoot, pathVal)
						if jErr == nil {
							normKey := strings.TrimRight(strings.ToLower(childURL), "/")
							if !c.seenURLs[normKey] {
								c.seenURLs[normKey] = true
								c.DiscoveredJobs = append(c.DiscoveredJobs, engine.Job{
									URL:    childURL,
									Origin: engine.OriginRobots,
								})
							}
						}
					}
				}
			}
		}
		_ = robotsBody.Close()
	}

	// 2. Discover & Parse sitemaps
	defaultSitemap := fmt.Sprintf("%s://%s/sitemap.xml", u.Scheme, u.Host)
	var startURLs []string
	startURLs = append(startURLs, defaultSitemap)
	for _, s := range robotsSitemaps {
		sURL, sErr := url.Parse(s)
		if sErr == nil && sURL.Host == u.Host {
			norm := strings.TrimRight(strings.ToLower(s), "/")
			if norm != strings.TrimRight(strings.ToLower(defaultSitemap), "/") {
				startURLs = append(startURLs, s)
			}
		}
	}

	disc, err := sitemap.NewDiscoverer(c.Client, c.Cache, hostRoot)
	if err == nil {
		disc.Discover(ctx, startURLs, func(candidatePath string, origin string) {
			c.SitemapDiscovered = true
			c.SitemapPaths = append(c.SitemapPaths, candidatePath)

			parsedCand, pErr := url.Parse(candidatePath)
			if pErr != nil {
				return
			}
			parsedCand.Fragment = ""
			childURL := hostRootURL.ResolveReference(parsedCand).String()
			normKey := strings.TrimRight(strings.ToLower(childURL), "/")
			if !c.seenURLs[normKey] {
				c.seenURLs[normKey] = true
				c.DiscoveredJobs = append(c.DiscoveredJobs, engine.Job{
					URL:    childURL,
					Origin: origin,
				})
			}
		})
	}

	// 3. Tech Detection for target host
	if c.Cache != nil {
		if fp := c.Cache.Get(u.Host); fp != nil {
			matcher := fingerprint.NewMatcher()
			for _, tech := range matcher.Match(fp) {
				if tech.Name == "Laravel" {
					c.LaravelDetected = true
				}
				if tech.Name == "WordPress" {
					c.WPDetected = true
				}
			}
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

	// 4. Build lookup maps for priority scoring
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

// DetectAndInjectFrameworks performs technology detection for a host and appends framework paths.
func (c *Collector) DetectAndInjectFrameworks(hostRoot string, host string) []engine.Job {
	if c.Cache == nil {
		return nil
	}

	fp := c.Cache.Get(host)
	if fp == nil {
		return nil
	}

	isLaravel := false
	isWP := false
	isExpress := false

	matcher := fingerprint.NewMatcher()
	for _, tech := range matcher.Match(fp) {
		if tech.Name == "Laravel" {
			isLaravel = true
		}
		if tech.Name == "WordPress" {
			isWP = true
		}
	}
	for _, sig := range fp.Signals() {
		val := strings.ToLower(sig.Value)
		src := strings.ToLower(sig.Source)
		if strings.Contains(val, "laravel") {
			isLaravel = true
		}
		if strings.Contains(val, "wordpress") || strings.Contains(src, "wordpress") {
			isWP = true
		}
		if strings.Contains(val, "express") {
			isExpress = true
		}
	}

	if isLaravel {
		c.LaravelDetected = true
	}
	if isWP {
		c.WPDetected = true
	}
	if isExpress {
		c.ExpressDetected = true
	}

	var newJobs []engine.Job
	if isLaravel && !c.injectedFrameworks[host+":laravel"] {
		c.injectedFrameworks[host+":laravel"] = true
		laravelPaths := []string{".env", "artisan", "storage/", "bootstrap/", "vendor/"}
		for _, p := range laravelPaths {
			childURL, err := wordlist.Join(hostRoot, p)
			if err == nil {
				normKey := strings.TrimRight(strings.ToLower(childURL), "/")
				if !c.seenURLs[normKey] {
					c.seenURLs[normKey] = true
					j := engine.Job{URL: childURL, Origin: "adaptive"}
					c.DiscoveredJobs = append(c.DiscoveredJobs, j)
					newJobs = append(newJobs, j)
				}
			}
		}
	}

	if isWP && !c.injectedFrameworks[host+":wordpress"] {
		c.injectedFrameworks[host+":wordpress"] = true
		wpPaths := []string{"wp-admin/", "wp-content/", "wp-includes/", "wp-login.php", "xmlrpc.php"}
		for _, p := range wpPaths {
			childURL, err := wordlist.Join(hostRoot, p)
			if err == nil {
				normKey := strings.TrimRight(strings.ToLower(childURL), "/")
				if !c.seenURLs[normKey] {
					c.seenURLs[normKey] = true
					j := engine.Job{URL: childURL, Origin: "adaptive"}
					c.DiscoveredJobs = append(c.DiscoveredJobs, j)
					newJobs = append(newJobs, j)
				}
			}
		}
	}

	if isExpress && !c.injectedFrameworks[host+":express"] {
		c.injectedFrameworks[host+":express"] = true
		expressPaths := []string{"api/", "uploads/", "assets/", "static/"}
		for _, p := range expressPaths {
			childURL, err := wordlist.Join(hostRoot, p)
			if err == nil {
				normKey := strings.TrimRight(strings.ToLower(childURL), "/")
				if !c.seenURLs[normKey] {
					c.seenURLs[normKey] = true
					j := engine.Job{URL: childURL, Origin: "adaptive"}
					c.DiscoveredJobs = append(c.DiscoveredJobs, j)
					newJobs = append(newJobs, j)
				}
			}
		}
	}

	return newJobs
}
