package recursion

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/robots"
	"github.com/unsubble/searchit/internal/sitemap"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/wordlist"
	"golang.org/x/time/rate"
)

// Manager orchestrates recursive directory scanning.
// It owns the frontier, the visited set, and all traversal decisions.
// Workers remain stateless execution units — they never know recursion exists.
type Manager struct {
	client           *http.Client
	exclude          status.Filters
	reader           wordlist.Reader
	strategy         Strategy
	maxDepth         uint16
	recurseOn        status.Filters
	normalizePaths   bool
	collapseSlashes  bool
	includeSize      size.Filters
	excludeSize      size.Filters
	includeHeaders   []engine.HeaderFilter
	excludeHeaders   []engine.HeaderFilter
	delay            time.Duration
	limiter          *rate.Limiter
	stats            *stats.Collector
	fingerprintCache *fingerprint.Cache
}

func NewManager(
	client *http.Client,
	exclude status.Filters,
	reader wordlist.Reader,
	strategy Strategy,
	maxDepth uint16,
	recurseOn status.Filters,
	normalizePaths bool,
	collapseSlashes bool,
	includeSize size.Filters,
	excludeSize size.Filters,
	includeHeaders []engine.HeaderFilter,
	excludeHeaders []engine.HeaderFilter,
	delay time.Duration,
	limiter *rate.Limiter,
	fingerprintCache *fingerprint.Cache,
) *Manager {
	return &Manager{
		client:           client,
		exclude:          exclude,
		reader:           reader,
		strategy:         strategy,
		maxDepth:         maxDepth,
		recurseOn:        recurseOn,
		normalizePaths:   normalizePaths,
		collapseSlashes:  collapseSlashes,
		includeSize:      includeSize,
		excludeSize:      excludeSize,
		includeHeaders:   includeHeaders,
		excludeHeaders:   excludeHeaders,
		delay:            delay,
		limiter:          limiter,
		fingerprintCache: fingerprintCache,
	}
}

// SetStats sets the statistics collector for the manager.
func (m *Manager) SetStats(c *stats.Collector) {
	m.stats = c
}

// Run performs a recursive scan starting from the given seed URLs.
// It feeds the worker pool, consumes results, and expands the frontier
// for any result that satisfies ShouldRecurse and has not been visited.
//
// The returned channel is closed when all traversal is complete.
// Cancelling ctx stops the scan at the next scheduling boundary.
func (m *Manager) Run(ctx context.Context, seeds []string, workers int) <-chan engine.Result {
	out := make(chan engine.Result, workers)

	go func() {
		defer close(out)

		frontier := NewFrontier(m.strategy)
		visited := make(map[string]struct{})

		for _, u := range seeds {
			key := normalizeURL(u)
			if _, seen := visited[key]; !seen {
				visited[key] = struct{}{}
				frontier.Push(engine.Job{URL: u, Depth: 0, Origin: engine.OriginProfile})
			}
		}

		if len(seeds) > 0 {
			robotsSitemaps := m.discoverRobots(ctx, seeds[0], frontier, visited)
			m.discoverSitemaps(ctx, seeds[0], robotsSitemaps, frontier, visited)
		}

		if m.stats != nil {
			m.stats.SetQueuedJobs(int64(frontier.Len()))
		}

		jobs := make(chan engine.Job, workers)
		results := engine.Start(
			ctx,
			m.client,
			m.exclude,
			m.includeSize,
			m.excludeSize,
			m.includeHeaders,
			m.excludeHeaders,
			workers,
			m.delay,
			m.limiter,
			jobs,
			m.stats,
		)

		// pending counts jobs dispatched to workers but not yet returned.
		// The loop ends when the frontier is empty and no in-flight work remains.
		pending := 0

		for frontier.Len() > 0 || pending > 0 {
			if m.stats != nil {
				m.stats.SetQueuedJobs(int64(frontier.Len()))
			}

			if frontier.Len() > 0 {
				next, _ := frontier.Peek()

				select {
				case <-ctx.Done():
					// Drain pending results before exiting so workers can finish
					// and the results channel closes without goroutine leaks.
					close(jobs)
					for range results {
					}
					return

				case jobs <- next:
					frontier.Pop()
					pending++

				case result, ok := <-results:
					if !ok {
						return
					}

					pending--
					m.handleResult(ctx, result, frontier, visited, out)
				}
			} else {
				// Frontier empty but workers still running; block until a result
				// arrives to avoid a busy-wait spin.
				select {
				case <-ctx.Done():
					close(jobs)
					for range results {
					}
					return
				case result, ok := <-results:
					if !ok {
						return
					}
					pending--
					m.handleResult(ctx, result, frontier, visited, out)
				}
			}
		}

		close(jobs)
		if m.stats != nil {
			m.stats.SetQueuedJobs(0)
		}
		// Drain any results that arrived after the last pending decrement.
		for result := range results {
			m.handleResult(ctx, result, frontier, visited, out)
		}
	}()

	return out
}

// handleResult forwards the result to the output channel and, if the result
// qualifies for recursion, generates child jobs from the wordlist.
func (m *Manager) handleResult(
	ctx context.Context,
	result engine.Result,
	frontier *Frontier,
	visited map[string]struct{},
	out chan<- engine.Result,
) {
	if !result.Accepted {
		return
	}

	select {
	case out <- result:
	case <-ctx.Done():
		return
	}

	if result.Depth >= m.maxDepth {
		return
	}
	if !m.recurseOn.Match(result.StatusCode) {
		return
	}

	words := make(chan string, wordlist.DefaultWordBuffer)
	readErr := make(chan error, 1)
	go func() {
		defer close(words)
		readErr <- m.reader.Read(ctx, words)
	}()

	for word := range words {
		cleaned, ok := wordlist.CleanWord(word, m.normalizePaths, m.collapseSlashes)
		if !ok {
			continue
		}
		childURL, err := wordlist.Join(result.URL, cleaned)
		if err != nil {
			continue
		}
		key := normalizeURL(childURL)
		if _, seen := visited[key]; seen {
			continue
		}
		visited[key] = struct{}{}
		frontier.Push(engine.Job{URL: childURL, Depth: result.Depth + 1, Origin: engine.OriginWordlist})
	}
	if err := <-readErr; err != nil && err != context.Canceled {
		// Reader failures during recursion do not invalidate already-scheduled children.
	}
}

// normalizeURL strips trailing slashes so /admin and /admin/ map to the same key.
func normalizeURL(u string) string {
	return strings.TrimRight(u, "/")
}

// discoverRobots downloads, parses, and enqueues robots.txt rules as high-priority seeds.
// It returns any Sitemap URLs discovered in the robots.txt file.
func (m *Manager) discoverRobots(ctx context.Context, targetURL string, frontier *Frontier, visited map[string]struct{}) []string {
	var sitemaps []string

	body, _, err := robots.Download(ctx, m.client, targetURL)
	if err != nil {
		return nil
	}
	defer body.Close()

	directives, err := robots.Parse(body)
	if err != nil {
		return nil
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		return nil
	}
	hostRoot := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	hostRootURL, _ := url.Parse(hostRoot)

	var fp *fingerprint.Fingerprint
	if m.fingerprintCache != nil {
		fp = m.fingerprintCache.GetOrCreate(u.Host)
	}

	for _, dir := range directives {
		if dir.Type == robots.Sitemap {
			sitemapURL, err := url.Parse(dir.Path)
			if err != nil {
				continue
			}
			resolvedSitemap := hostRootURL.ResolveReference(sitemapURL).String()
			sitemaps = append(sitemaps, resolvedSitemap)
			continue
		}

		if fp != nil {
			source := "robots:allow"
			if dir.Type == robots.Disallow {
				source = "robots:disallow"
			}
			fp.AddSignal(fingerprint.Signal{
				Source:     source,
				Value:      dir.Path,
				Confidence: fingerprint.Confidence(1.0),
			})
		}

		pathVal := strings.TrimSpace(dir.Path)
		if idx := strings.IndexAny(pathVal, "*$"); idx != -1 {
			pathVal = pathVal[:idx]
		}
		pathVal = strings.TrimSpace(pathVal)
		if pathVal == "" {
			continue
		}
		if !strings.HasPrefix(pathVal, "/") {
			pathVal = "/" + pathVal
		}

		childURL, err := wordlist.Join(hostRoot, pathVal)
		if err != nil {
			continue
		}

		key := normalizeURL(childURL)
		if _, seen := visited[key]; seen {
			continue
		}
		visited[key] = struct{}{}
		frontier.PushFront(engine.Job{URL: childURL, Depth: 0, Origin: engine.OriginRobots})
	}

	return sitemaps
}

// discoverSitemaps triggers sitemap crawling starting with default locations and sitemaps from robots.txt.
func (m *Manager) discoverSitemaps(ctx context.Context, targetURL string, robotsSitemaps []string, frontier *Frontier, visited map[string]struct{}) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return
	}
	defaultSitemap := fmt.Sprintf("%s://%s/sitemap.xml", u.Scheme, u.Host)

	// Build list of unique starting sitemap URLs
	var startURLs []string
	startURLs = append(startURLs, defaultSitemap)
	for _, s := range robotsSitemaps {
		sURL, err := url.Parse(s)
		if err != nil {
			continue
		}
		if sURL.Host != u.Host {
			continue // SSRF prevention: ignore sitemaps located on foreign hosts
		}
		norm := strings.TrimRight(strings.ToLower(s), "/")
		if norm != strings.TrimRight(strings.ToLower(defaultSitemap), "/") {
			startURLs = append(startURLs, s)
		}
	}

	disc, err := sitemap.NewDiscoverer(m.client, m.fingerprintCache, targetURL)
	if err != nil {
		return
	}

	hostRoot := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	hostRootURL, _ := url.Parse(hostRoot)

	disc.Discover(ctx, startURLs, func(candidatePath string, origin string) {
		parsedCand, err := url.Parse(candidatePath)
		if err != nil {
			return
		}
		childURL := hostRootURL.ResolveReference(parsedCand).String()

		key := normalizeURL(childURL)
		if _, seen := visited[key]; seen {
			return
		}
		visited[key] = struct{}{}
		frontier.PushFront(engine.Job{URL: childURL, Depth: 0, Origin: origin})
	})
}
