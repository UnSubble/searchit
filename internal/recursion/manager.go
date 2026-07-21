package recursion

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/unsubble/searchit/internal/adaptive/prioritizer"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/extensions"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/robots"
	"github.com/unsubble/searchit/internal/sitemap"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/wildcard"
	"github.com/unsubble/searchit/internal/wordlist"
	"golang.org/x/time/rate"
)

// Manager orchestrates recursive directory scanning.
// It owns the frontier, the visited set, and all traversal decisions.
// Workers remain stateless execution units — they never know recursion exists.
type Manager struct {
	client           *http.Client
	fs               *filter.FilterSuite
	reader           wordlist.Reader
	extensions       []string
	strategy         Strategy
	maxDepth         uint16
	recurseOn        status.Filters
	normalizePaths   bool
	collapseSlashes  bool
	includeHeaders   []engine.HeaderFilter
	excludeHeaders   []engine.HeaderFilter
	delay            time.Duration
	limiter          *rate.Limiter
	stats            *stats.Collector
	fingerprintCache *fingerprint.Cache
	wildcardDetector *wildcard.Detector
	disableWildcard  bool

	// Request manipulation fields
	method  string
	body    []byte
	headers http.Header
	cookies []*http.Cookie

	// Adaptive summary tracking fields
	LaravelDetected     bool
	WPDetected          bool
	ExpressDetected     bool
	RobotsDiscovered    bool
	SitemapDiscovered   bool
	DFSCount            int
	BFSCount            int
	EagerCount          int
	HighPriorityCount   int
	MediumPriorityCount int
	LowPriorityCount    int
}

// SetRequestManipulation configures custom outbound request templates for scanning.
func (m *Manager) SetRequestManipulation(method string, body []byte, headers http.Header, cookies []*http.Cookie) {
	m.method = method
	m.body = body
	m.headers = headers
	m.cookies = cookies
}

// SetFilterSuite configures response filters.
func (m *Manager) SetFilterSuite(fs *filter.FilterSuite) {
	m.fs = fs
}

// SetExtensions configures extension variants for recursion candidate generation.
func (m *Manager) SetExtensions(exts []string) {
	m.extensions = exts
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
	fs, _ := filter.NewFilterSuite("", exclude.String(), includeSize.String(), excludeSize.String(), nil, nil, nil, nil)

	return &Manager{
		client:           client,
		fs:               fs,
		reader:           reader,
		strategy:         strategy,
		maxDepth:         maxDepth,
		recurseOn:        recurseOn,
		normalizePaths:   normalizePaths,
		collapseSlashes:  collapseSlashes,
		includeHeaders:   includeHeaders,
		excludeHeaders:   excludeHeaders,
		delay:            delay,
		limiter:          limiter,
		fingerprintCache: fingerprintCache,
		wildcardDetector: wildcard.NewDetector(),
	}
}

// SetStats sets the statistics collector for the manager.
func (m *Manager) SetStats(c *stats.Collector) {
	m.stats = c
}

// SetDisableWildcard enables or disables automatic wildcard detection.
func (m *Manager) SetDisableWildcard(disable bool) {
	m.disableWildcard = disable
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
		defer func() {
			atomic.AddInt64(&stats.GlobalInstrumentation.SchedulerExit, 1)
			stats.GlobalInstrumentation.LogEvent("manager exit")
			close(out)
		}()

		frontier := NewFrontier(m.strategy)
		visited := make(map[string]struct{})
		injectedLaravel := make(map[string]bool)
		injectedWordPress := make(map[string]bool)
		injectedExpress := make(map[string]bool)

		for _, u := range seeds {
			key := normalizeURL(u)
			if _, seen := visited[key]; !seen {
				visited[key] = struct{}{}
				atomic.AddInt64(&stats.GlobalInstrumentation.JobsAccepted, 1)
				atomic.AddInt64(&stats.GlobalInstrumentation.JobsProduced, 1)
				m.MediumPriorityCount++
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
			m.fs,
			m.includeHeaders,
			m.excludeHeaders,
			workers,
			m.delay,
			m.limiter,
			m.method,
			m.body,
			m.headers,
			m.cookies,
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
					stats.GlobalInstrumentation.LogEvent("context cancellation")
					stats.GlobalInstrumentation.LogEvent("jobs channel close")
					close(jobs)
					for result := range results {
						atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
						if result.Accepted {
							atomic.AddInt64(&stats.GlobalInstrumentation.ResultsAccepted, 1)
						} else {
							atomic.AddInt64(&stats.GlobalInstrumentation.ResultsRejected, 1)
						}
					}
					return

				case jobs <- next:
					atomic.AddInt64(&stats.GlobalInstrumentation.JobsDispatched, 1)
					atomic.AddInt64(&stats.GlobalInstrumentation.JobsSubmitted, 1)
					frontier.Pop()
					pending++

				case result, ok := <-results:
					if !ok {
						return
					}

					atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
					pending--
					m.handleResult(ctx, result, frontier, visited, injectedLaravel, injectedWordPress, injectedExpress, out)
				}
			} else {
				// Frontier empty but workers still running; block until a result
				// arrives to avoid a busy-wait spin.
				select {
				case <-ctx.Done():
					stats.GlobalInstrumentation.LogEvent("context cancellation")
					stats.GlobalInstrumentation.LogEvent("jobs channel close")
					close(jobs)
					for result := range results {
						atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
						if result.Accepted {
							atomic.AddInt64(&stats.GlobalInstrumentation.ResultsAccepted, 1)
						} else {
							atomic.AddInt64(&stats.GlobalInstrumentation.ResultsRejected, 1)
						}
					}
					return
				case result, ok := <-results:
					if !ok {
						return
					}
					atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
					pending--
					m.handleResult(ctx, result, frontier, visited, injectedLaravel, injectedWordPress, injectedExpress, out)
				}
			}
		}

		atomic.StoreInt64(&stats.GlobalInstrumentation.JobsRemaining, int64(frontier.Len()))
		stats.GlobalInstrumentation.LogEvent("jobs channel close")
		close(jobs)
		if m.stats != nil {
			m.stats.SetQueuedJobs(0)
		}
		// Drain any results that arrived after the last pending decrement.
		for result := range results {
			atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
			m.handleResult(ctx, result, frontier, visited, injectedLaravel, injectedWordPress, injectedExpress, out)
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
	injectedLaravel map[string]bool,
	injectedWordPress map[string]bool,
	injectedExpress map[string]bool,
	out chan<- engine.Result,
) {
	if !result.Accepted {
		atomic.AddInt64(&stats.GlobalInstrumentation.ResultsRejected, 1)
		return
	}

	// Wildcard detection signature check
	if !m.disableWildcard {
		var host string
		if u, err := url.Parse(result.URL); err == nil {
			host = u.Host
		}
		sig := wildcard.Signature{
			StatusCode: result.StatusCode,
			BodyHash:   result.BodyHash,
			BodySize:   result.Length,
		}
		wasWildcardBefore := m.wildcardDetector.IsWildcard(host, result.Depth, sig)
		_, active := m.wildcardDetector.Add(host, result.Depth, sig)
		if active {
			if !wasWildcardBefore {
				atomic.AddInt64(&stats.GlobalInstrumentation.WildcardsDetected, 1)
			}
			atomic.AddInt64(&stats.GlobalInstrumentation.RequestsFiltered, 1)
			atomic.AddInt64(&stats.GlobalInstrumentation.ResultsRejected, 1)
			return
		}
	}

	atomic.AddInt64(&stats.GlobalInstrumentation.ResultsAccepted, 1)

	// If it's a redirect response (3xx), enqueue the destination URL to be scanned and return
	if result.RedirectURL != "" && result.StatusCode >= 300 && result.StatusCode < 400 {
		select {
		case out <- result:
		case <-ctx.Done():
			stats.GlobalInstrumentation.LogEvent("context cancellation")
			return
		}

		if result.Depth >= m.maxDepth {
			return
		}

		key := normalizeURL(result.RedirectURL)
		if _, seen := visited[key]; !seen {
			visited[key] = struct{}{}
			atomic.AddInt64(&stats.GlobalInstrumentation.JobsAccepted, 1)
			atomic.AddInt64(&stats.GlobalInstrumentation.JobsProduced, 1)
			frontier.Push(engine.Job{URL: result.RedirectURL, Depth: result.Depth, Origin: "redirect"})
		}
		return
	}

	select {
	case out <- result:
	case <-ctx.Done():
		stats.GlobalInstrumentation.LogEvent("context cancellation")
		return
	}

	if result.Depth >= m.maxDepth {
		return
	}

	parentURL := result.URL
	if result.RedirectURL != "" {
		parentURL = result.RedirectURL
	}

	// Technology detection and path injection (Laravel, WordPress, Express)
	if m.fingerprintCache != nil {
		parsed, err := url.Parse(parentURL)
		if err == nil {
			host := parsed.Host
			fp := m.fingerprintCache.Get(host)
			if fp != nil {
				matcher := fingerprint.NewMatcher()
				isLaravel := false
				isWP := false
				isExpress := false
				for _, tech := range matcher.Match(fp) {
					if tech.Name == "Laravel" {
						isLaravel = true
					}
					if tech.Name == "WordPress" {
						isWP = true
					}
				}
				for _, sig := range fp.Signals() {
					if strings.Contains(strings.ToLower(sig.Value), "express") {
						isExpress = true
					}
				}

				scheme := "http"
				if parsed.Scheme != "" {
					scheme = parsed.Scheme
				}
				baseURL := fmt.Sprintf("%s://%s", scheme, host)

				if isLaravel && !injectedLaravel[host] {
					injectedLaravel[host] = true
					m.LaravelDetected = true
					laravelPaths := []string{".env", "artisan", "storage/", "bootstrap/", "vendor/"}
					for _, p := range laravelPaths {
						childURL, err := wordlist.Join(baseURL, p)
						if err == nil {
							key := normalizeURL(childURL)
							if _, seen := visited[key]; !seen {
								visited[key] = struct{}{}
								atomic.AddInt64(&stats.GlobalInstrumentation.JobsAccepted, 1)
								atomic.AddInt64(&stats.GlobalInstrumentation.JobsProduced, 1)
								m.HighPriorityCount++
								frontier.PushFront(engine.Job{
									URL:    childURL,
									Depth:  result.Depth + 1,
									Origin: "adaptive",
								})
							}
						}
					}
				}

				if isWP && !injectedWordPress[host] {
					injectedWordPress[host] = true
					m.WPDetected = true
					wpPaths := []string{"wp-admin/", "wp-content/", "wp-includes/", "wp-login.php", "xmlrpc.php"}
					for _, p := range wpPaths {
						childURL, err := wordlist.Join(baseURL, p)
						if err == nil {
							key := normalizeURL(childURL)
							if _, seen := visited[key]; !seen {
								visited[key] = struct{}{}
								atomic.AddInt64(&stats.GlobalInstrumentation.JobsAccepted, 1)
								atomic.AddInt64(&stats.GlobalInstrumentation.JobsProduced, 1)
								m.HighPriorityCount++
								frontier.PushFront(engine.Job{
									URL:    childURL,
									Depth:  result.Depth + 1,
									Origin: "adaptive",
								})
							}
						}
					}
				}

				if isExpress && !injectedExpress[host] {
					injectedExpress[host] = true
					m.ExpressDetected = true
					expressPaths := []string{"api/", "uploads/", "assets/", "static/"}
					for _, p := range expressPaths {
						childURL, err := wordlist.Join(baseURL, p)
						if err == nil {
							key := normalizeURL(childURL)
							if _, seen := visited[key]; !seen {
								visited[key] = struct{}{}
								atomic.AddInt64(&stats.GlobalInstrumentation.JobsAccepted, 1)
								atomic.AddInt64(&stats.GlobalInstrumentation.JobsProduced, 1)
								m.HighPriorityCount++
								frontier.PushFront(engine.Job{
									URL:    childURL,
									Depth:  result.Depth + 1,
									Origin: "adaptive",
								})
							}
						}
					}
				}
			}
		}
	}

	if m.recurseOn.Match(result.StatusCode) {
		if m.strategy == DFS {
			m.DFSCount++
		} else {
			m.BFSCount++
		}
	} else {
		return
	}

	// Process HTML-extracted links (same-host only) as high-priority jobs
	if len(result.Links) > 0 {
		parentParsed, err := url.Parse(parentURL)
		if err == nil {
			for _, link := range result.Links {
				linkParsed, err := url.Parse(link)
				if err != nil {
					continue
				}
				resolved := parentParsed.ResolveReference(linkParsed)
				if resolved.Host == parentParsed.Host {
					resolvedStr := resolved.String()
					key := normalizeURL(resolvedStr)
					if _, seen := visited[key]; !seen {
						visited[key] = struct{}{}
						atomic.AddInt64(&stats.GlobalInstrumentation.JobsAccepted, 1)
						atomic.AddInt64(&stats.GlobalInstrumentation.JobsProduced, 1)
						m.HighPriorityCount++
						frontier.PushFront(engine.Job{
							URL:    resolvedStr,
							Depth:  result.Depth + 1,
							Origin: engine.OriginHTML,
						})
					}
				}
			}
		}
	}

	var parentPath []string
	var host string
	parentParsed, err := url.Parse(parentURL)
	if err == nil {
		host = parentParsed.Host
		segments := strings.Split(parentParsed.Path, "/")
		for _, seg := range segments {
			if seg != "" {
				parentPath = append(parentPath, seg)
			}
		}
	}

	parentResContentType := result.Headers.Get("Content-Type")

	var prioritizedSegments = make(map[string]bool)
	var prioritizedPaths = make(map[string]bool)
	var laravel, wp, express bool

	if m.fingerprintCache != nil && host != "" {
		fp := m.fingerprintCache.Get(host)
		if fp != nil {
			matcher := fingerprint.NewMatcher()
			for _, tech := range matcher.Match(fp) {
				if tech.Name == "Laravel" {
					laravel = true
				}
				if tech.Name == "WordPress" {
					wp = true
				}
			}
			for _, sig := range fp.Signals() {
				val := strings.ToLower(sig.Value)
				src := strings.ToLower(sig.Source)
				if strings.Contains(val, "express") {
					express = true
				}
				if strings.HasPrefix(src, "robots:") || strings.HasPrefix(src, "sitemap:") {
					p := strings.Trim(sig.Value, "/")
					prioritizedPaths[p] = true
					parts := strings.Split(p, "/")
					for _, part := range parts {
						part = strings.TrimSpace(part)
						if part != "" {
							prioritizedSegments[strings.ToLower(part)] = true
						}
					}
				}
			}
		}
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
		variants := extensions.GenerateVariants(cleaned, m.extensions)
		for _, variant := range variants {
			childURL, err := wordlist.Join(parentURL, variant)
			if err != nil {
				continue
			}
			key := normalizeURL(childURL)
			if _, seen := visited[key]; seen {
				continue
			}

			// Adaptive scoring
			score := 0
			isAdaptive := m.fingerprintCache != nil
			if isAdaptive {
				sigs := prioritizer.CalculateSignals(
					variant,
					parentPath,
					int(result.Depth+1),
					parentResContentType,
					prioritizedSegments,
					prioritizedPaths,
					laravel,
					wp,
					express,
				)
				score = prioritizer.GetScore(sigs)
			}

			visited[key] = struct{}{}
			atomic.AddInt64(&stats.GlobalInstrumentation.JobsAccepted, 1)
			atomic.AddInt64(&stats.GlobalInstrumentation.JobsProduced, 1)

			if isAdaptive && score > 50 {
				m.HighPriorityCount++
				frontier.PushFront(engine.Job{URL: childURL, Depth: result.Depth + 1, Origin: engine.OriginWordlist})
			} else {
				m.LowPriorityCount++
				frontier.Push(engine.Job{URL: childURL, Depth: result.Depth + 1, Origin: engine.OriginWordlist})
			}
		}
	}
	// Reader failures during recursion do not invalidate already-scheduled children.
	<-readErr
}

// normalizeURL strips trailing slashes and fragments to normalize directory matches.
func normalizeURL(u string) string {
	if strings.Contains(u, "?") || strings.Contains(u, "#") {
		parsed, err := url.Parse(u)
		if err != nil {
			return strings.TrimRight(u, "/")
		}
		parsed.Fragment = ""
		p := parsed.Path
		if len(p) > 1 && strings.HasSuffix(p, "/") {
			p = p[:len(p)-1]
		}
		parsed.Path = p
		return parsed.String()
	}

	if strings.HasSuffix(u, "/") {
		slashes := 0
		for i := 0; i < len(u); i++ {
			if u[i] == '/' {
				slashes++
			}
		}
		if slashes > 3 {
			return u[:len(u)-1]
		}
	}
	return u
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

	m.RobotsDiscovered = true
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
				Source: source,
				Value:  dir.Path,
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
		atomic.AddInt64(&stats.GlobalInstrumentation.JobsAccepted, 1)
		atomic.AddInt64(&stats.GlobalInstrumentation.JobsProduced, 1)
		m.HighPriorityCount++
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
		m.SitemapDiscovered = true
		parsedCand, err := url.Parse(candidatePath)
		if err != nil {
			return
		}
		parsedCand.Fragment = "" // Strip fragments to prevent double-scheduling
		childURL := hostRootURL.ResolveReference(parsedCand).String()

		key := normalizeURL(childURL)
		if _, seen := visited[key]; seen {
			return
		}
		visited[key] = struct{}{}
		atomic.AddInt64(&stats.GlobalInstrumentation.JobsAccepted, 1)
		atomic.AddInt64(&stats.GlobalInstrumentation.JobsProduced, 1)
		m.HighPriorityCount++
		frontier.PushFront(engine.Job{URL: childURL, Depth: 0, Origin: origin})
	})
}

func (m *Manager) GetAdaptiveMetrics() (techs []string, discoveries []string, dfs, bfs, eager, high, med, low int) {
	if m.LaravelDetected {
		techs = append(techs, "Laravel")
	}
	if m.WPDetected {
		techs = append(techs, "WordPress")
	}
	if m.ExpressDetected {
		techs = append(techs, "Express")
	}
	if m.RobotsDiscovered {
		discoveries = append(discoveries, "robots.txt")
	}
	if m.SitemapDiscovered {
		discoveries = append(discoveries, "sitemap.xml")
	}
	return techs, discoveries, m.DFSCount, m.BFSCount, m.EagerCount, m.HighPriorityCount, m.MediumPriorityCount, m.LowPriorityCount
}
