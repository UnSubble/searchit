package recursion

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unsubble/searchit/internal/adaptive"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/wildcard"
	"github.com/unsubble/searchit/internal/wordlist"
	"golang.org/x/time/rate"
)

// Architectural invariant:
//
// Traversal decisions MUST NEVER depend on user-facing display filters.
//
// Traversal is driven only by:
//
//   - recurse-on
//   - wildcard detection
//   - redirect policy
//   - visited state
//   - recursion depth
//   - context cancellation
//
// Display filters affect only:
//
//   - reported findings (via m.displayFS)
//   - formatter output
//   - displayed statistics
//
// Violating this separation is a bug.
// Manager orchestrates recursive directory scanning.
// It owns the frontier, the visited set, and all traversal decisions.
// Workers remain stateless execution units — they never know recursion exists.
type Manager struct {
	client            *http.Client
	fs                *filter.FilterSuite   // traversal filter — drives crawl decisions; never user-facing
	displayFS         *filter.FilterSuite   // user-facing display filter — only affects onResult reporting
	displayIncHeaders []engine.HeaderFilter // user-facing display filter
	displayExcHeaders []engine.HeaderFilter // user-facing display filter
	reader            wordlist.Reader
	extensions        []string
	strategy          Strategy
	maxDepth          uint16

	recurseOn        status.Filters
	normalizePaths   bool
	collapseSlashes  bool
	delay            time.Duration
	limiter          *rate.Limiter
	stats            *stats.Collector
	fingerprintCache *fingerprint.Cache
	adaptiveEngine   *adaptive.Engine
	PauseBlocker     func(context.Context) error
	wildcardDetector *wildcard.Detector
	disableWildcard  bool
	warningHandler   func(string)

	// Request manipulation fields
	method    string
	body      []byte
	headers   http.Header
	cookieStr string

	// Adaptive summary tracking fields
	DFSCount            int
	BFSCount            int
	EagerCount          int
	HighPriorityCount   int
	MediumPriorityCount int
	LowPriorityCount    int
	entriesPerDir       int64
}

// SetAdaptiveEngine configures the unified AdaptiveEngine for recursion.
func (m *Manager) SetAdaptiveEngine(eng *adaptive.Engine) {
	m.adaptiveEngine = eng
}

// RemainingSubtreeWork returns the theoretical request count for unvisited descendant depths below currentDepth.
func (m *Manager) RemainingSubtreeWork(currentDepth uint16) int64 {
	if currentDepth >= m.maxDepth {
		return 0
	}
	remainingDepths := int64(m.maxDepth) - int64(currentDepth)
	return remainingDepths * m.entriesPerDir
}

// SetRequestManipulation configures custom outbound request templates for scanning.
func (m *Manager) SetRequestManipulation(method string, body []byte, headers http.Header, cookieStr string) {
	m.method = method
	m.body = body
	m.headers = headers
	m.cookieStr = cookieStr
}

// SetFilterSuite configures the user-facing display filter.
//
// The display filter determines which results are reported as findings via onResult.
// It does NOT replace the traversal filter (m.fs) that drives crawl/recursion decisions.
// This ensures that user-specified output filters (--fc, --mc, --fs, etc.) never prevent
// the crawler from discovering and recursing into directories.
func (m *Manager) SetFilterSuite(fs *filter.FilterSuite) {
	m.displayFS = fs
}

// SetDisplayHeaders sets the display-layer header filters.
func (m *Manager) SetDisplayHeaders(inc, exc []engine.HeaderFilter) {
	m.displayIncHeaders = inc
	m.displayExcHeaders = exc
}

// matchDisplayHeaders evaluates display header filters against the response headers.
func (m *Manager) matchDisplayHeaders(headers http.Header) bool {
	if m.displayIncHeaders == nil && m.displayExcHeaders == nil {
		return true
	}
	for _, f := range m.displayExcHeaders {
		if hasHeader(headers, f.Name, f.Value) {
			return false
		}
	}
	for _, f := range m.displayIncHeaders {
		if !hasHeader(headers, f.Name, f.Value) {
			return false
		}
	}
	return true
}

func hasHeader(headers http.Header, name, value string) bool {
	for k, values := range headers {
		if strings.EqualFold(k, name) {
			for _, val := range values {
				if strings.EqualFold(val, value) {
					return true
				}
			}
		}
	}
	return false
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
	delay time.Duration,
	limiter *rate.Limiter,
	fingerprintCache *fingerprint.Cache,
	entriesPerDir int64,
) *Manager {
	fs, _ := filter.NewFilterSuite("", exclude.String(), "", "", nil, nil, nil, nil)
	fs.ShowHeaders = true // ALWAYS extract headers so display layer can filter by them

	return &Manager{
		client:           client,
		fs:               fs,
		reader:           wordlist.NewSliceReader(reader),
		strategy:         strategy,
		maxDepth:         maxDepth,
		recurseOn:        recurseOn,
		normalizePaths:   normalizePaths,
		collapseSlashes:  collapseSlashes,
		delay:            delay,
		limiter:          limiter,
		fingerprintCache: fingerprintCache,
		entriesPerDir:    entriesPerDir,
		wildcardDetector: wildcard.NewDetector(),
		disableWildcard:  true,
	}
}

// SetStats sets the statistics collector for the manager.
func (m *Manager) SetStats(c *stats.Collector) {
	m.stats = c
	if c != nil {
		c.SetIsFinite(false)
	}
}

// SetDisableWildcard enables or disables automatic wildcard detection.
func (m *Manager) SetDisableWildcard(disable bool) {
	m.disableWildcard = disable
}

// SetWarningHandler sets a custom warning handler function.
func (m *Manager) SetWarningHandler(fn func(string)) {
	m.warningHandler = fn
}

// Run performs a recursive scan starting from the given seed URLs.
// It feeds the worker pool, consumes results, and expands the frontier
// for any result that satisfies ShouldRecurse and has not been visited.
//
// The returned channel is closed when all traversal is complete.
// Cancelling ctx stops the scan at the next scheduling boundary.
func (m *Manager) Run(
	ctx context.Context,
	drainCtx context.Context,
	seeds []string,
	workers int,
	onResult func(r engine.Result),
) error {
	var runErr error
	func() {
		defer func() {
			atomic.AddInt64(&stats.GlobalInstrumentation.SchedulerExit, 1)
			stats.GlobalInstrumentation.LogEvent("manager exit")
		}()

		frontier := NewFrontier(m.strategy)
		visited := make(map[string]struct{})
		injectedLaravel := make(map[string]bool)
		injectedWordPress := make(map[string]bool)
		injectedExpress := make(map[string]bool)

		for _, u := range seeds {
			if m.PauseBlocker != nil {
				if err := m.PauseBlocker(ctx); err != nil {
					return
				}
			}
			key := normalizeURL(u)
			if _, seen := visited[key]; !seen {
				visited[key] = struct{}{}
				m.MediumPriorityCount++
				frontier.Push(NewSliceGenerator([]engine.Job{{URL: u, Depth: 0, Origin: engine.OriginProfile}}))
			}
		}

		if len(seeds) > 0 {
			if m.adaptiveEngine == nil || (m.adaptiveEngine.TargetURL != "" && m.adaptiveEngine.TargetURL != seeds[0]) {
				m.adaptiveEngine = adaptive.NewEngine(seeds[0], m.client, m.fingerprintCache, true)
			}
		}

		if m.adaptiveEngine != nil {
			if m.adaptiveEngine.TargetURL == "" && len(seeds) > 0 {
				m.adaptiveEngine.TargetURL = seeds[0]
				m.adaptiveEngine.Collector.TargetURL = seeds[0]
			}
			_ = m.adaptiveEngine.Discover(ctx)
			discovered := m.adaptiveEngine.GetDiscoveredJobs()
			for _, job := range discovered {
				key := normalizeURL(job.URL)
				if _, seen := visited[key]; !seen {
					visited[key] = struct{}{}
					m.MediumPriorityCount++
					frontier.PushFront(NewSliceGenerator([]engine.Job{job}))
				}
			}
		}

		jobs := make(chan engine.Job, workers)
		var jobsOnce sync.Once
		closeJobs := func() {
			jobsOnce.Do(func() {
				stats.GlobalInstrumentation.LogEvent("jobs channel close")
				close(jobs)
			})
		}
		defer closeJobs()

		results := engine.Start(
			ctx,
			drainCtx,
			m.client,
			m.fs,
			nil, // incHeaders: display-only filters must not reach the traversal engine
			nil, // excHeaders: display-only filters must not reach the traversal engine
			workers,
			m.delay,
			m.limiter,
			m.method,
			m.body,
			m.headers,
			m.cookieStr,
			jobs,
			m.stats,
			m.PauseBlocker,
			engine.WorkerOptions{ExtractLinks: true, DeferDiscoveredAccounting: true},
		)

		// pending counts jobs dispatched to workers but not yet returned.
		// The loop ends when the frontier is empty and no in-flight work remains.
		pending := 0
		var maxDepthReached uint16 = 1

		var activeGenerator Generator
		var nextJob engine.Job
		var hasNextJob bool

		for (frontier.Len() > 0 || activeGenerator != nil || hasNextJob) || pending > 0 {

			if ctx.Err() != nil {
				stats.GlobalInstrumentation.LogEvent("context cancellation")
				closeJobs()
				for result := range results {
					atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
					_ = m.handleResult(context.Background(), result, frontier, &activeGenerator, visited, injectedLaravel, injectedWordPress, injectedExpress, onResult)
				}
				return
			}

			if activeGenerator == nil && frontier.Len() > 0 {
				activeGenerator, _ = frontier.Peek()
				frontier.Pop()
			}

			if !hasNextJob && activeGenerator != nil {
				nextJob, hasNextJob = activeGenerator.Next()
				if !hasNextJob {
					activeGenerator = nil
					continue
				}
			}

			if hasNextJob {

				select {
				case <-ctx.Done():
					// Drain pending results before exiting so workers can finish
					// and the results channel closes without goroutine leaks.
					stats.GlobalInstrumentation.LogEvent("context cancellation")
					closeJobs()
					for result := range results {
						atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
						_ = m.handleResult(context.Background(), result, frontier, &activeGenerator, visited, injectedLaravel, injectedWordPress, injectedExpress, onResult)
					}
					return

				case jobs <- nextJob:
					if nextJob.Depth > maxDepthReached {
						maxDepthReached = nextJob.Depth
					}
					if m.stats != nil {
						m.stats.RecordJobProduced()
					}
					atomic.AddInt64(&stats.GlobalInstrumentation.JobsDispatched, 1)
					atomic.AddInt64(&stats.GlobalInstrumentation.JobsSubmitted, 1)
					pending++
					hasNextJob = false

				case result, ok := <-results:
					if !ok {
						return
					}

					atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
					pending--
					if err := m.handleResult(ctx, result, frontier, &activeGenerator, visited, injectedLaravel, injectedWordPress, injectedExpress, onResult); err != nil {
						runErr = err
						return
					}
				}
			} else {
				// Frontier empty but workers still running; block until a result
				// arrives to avoid a busy-wait spin.
				select {
				case <-ctx.Done():
					stats.GlobalInstrumentation.LogEvent("context cancellation")
					closeJobs()
					for result := range results {
						atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
						_ = m.handleResult(context.Background(), result, frontier, &activeGenerator, visited, injectedLaravel, injectedWordPress, injectedExpress, onResult)
					}
					return
				case result, ok := <-results:
					if !ok {
						return
					}
					atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
					pending--
					if err := m.handleResult(ctx, result, frontier, &activeGenerator, visited, injectedLaravel, injectedWordPress, injectedExpress, onResult); err != nil {
						runErr = err
						return
					}
				}
			}
		}

		atomic.StoreInt64(&stats.GlobalInstrumentation.JobsRemaining, int64(frontier.Len()))
		closeJobs()

		// Drain any results that arrived after the last pending decrement.
		for result := range results {
			atomic.AddInt64(&stats.GlobalInstrumentation.ResultsConsumed, 1)
			if err := m.handleResult(ctx, result, frontier, &activeGenerator, visited, injectedLaravel, injectedWordPress, injectedExpress, onResult); err != nil {
				if runErr == nil {
					runErr = err
				}
			}
		}

		if ctx.Err() == nil && m.stats != nil {
			remaining := m.RemainingSubtreeWork(maxDepthReached)
			if remaining > 0 {
				m.stats.RecordSkipped(remaining)
			}
		}
	}()
	return runErr
}

// handleResult forwards the result to the output channel and, if the result
// qualifies for recursion, generates child jobs from the wordlist.
func (m *Manager) handleResult(
	ctx context.Context,
	result engine.Result,
	frontier *Frontier,
	activeGenerator *Generator,
	visited map[string]struct{},
	injectedLaravel map[string]bool,
	injectedWordPress map[string]bool,
	injectedExpress map[string]bool,
	onResult func(engine.Result),
) error {
	if !result.Accepted {
		atomic.AddInt64(&stats.GlobalInstrumentation.ResultsRejected, 1)
		return nil
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
			if m.stats != nil {
				m.stats.RecordWildcardFiltered()
			}
			return nil
		}
	}

	atomic.AddInt64(&stats.GlobalInstrumentation.ResultsAccepted, 1)

	reported := result
	if result.Err != nil {
		reported.Accepted = false
		if m.stats != nil {
			m.stats.RecordDisplayFiltered()
		}
	} else {
		if m.displayFS != nil {
			contentType := ""
			if result.Headers != nil {
				contentType = result.Headers.Get("Content-Type")
			}
			if !m.displayFS.MatchHeaders(result.StatusCode, result.Length, contentType) {
				reported.Accepted = false
				if m.stats != nil {
					m.stats.RecordDisplayFiltered()
				}
			}
		}

		if reported.Accepted && result.Headers != nil {
			if !m.matchDisplayHeaders(result.Headers) {
				reported.Accepted = false
				if m.stats != nil {
					m.stats.RecordDisplayFiltered()
				}
			}
		}
	}

	if reported.Accepted {
		if m.stats != nil {
			m.stats.RecordDiscovered()
		}
	}
	onResult(reported)

	if !m.recurseOn.Match(result.StatusCode) {
		if result.Depth == 0 && result.Err == nil && ctx.Err() == nil {
			msg := FormatRecurseWarning(result.StatusCode, m.recurseOn.String())
			if m.warningHandler != nil {
				m.warningHandler(msg)
			} else {
				fmt.Fprintln(os.Stderr, msg)
			}
		}
		return nil
	}

	// If it's a redirect response (3xx) to a different URL path, enqueue the destination URL to be scanned and return.
	// For trailing-slash self-redirects (e.g. /path -> /path/), continue down to generate directory candidates.
	if result.RedirectURL != "" && result.StatusCode >= 300 && result.StatusCode < 400 {
		if ctx.Err() != nil {
			stats.GlobalInstrumentation.LogEvent("context cancellation")
			return nil
		}

		if result.Depth >= m.maxDepth {
			return nil
		}

		key := normalizeURL(result.RedirectURL)
		parentKey := normalizeURL(result.URL)
		if key != parentKey {
			wasAlreadyFollowed := visited[key] != struct{}{} && (result.Length > 0 || (ctx != nil && ctx.Err() == nil))
			if _, seen := visited[key]; !seen {
				visited[key] = struct{}{}
				// If HTTP client already followed the redirect during client.Do, do not re-enqueue
				if result.Length <= 0 {
					frontier.Push(NewSliceGenerator([]engine.Job{{URL: result.RedirectURL, Depth: result.Depth, Origin: "redirect"}}))
					return nil
				}
			} else if !wasAlreadyFollowed {
				return nil
			}
		}
	}

	if ctx.Err() != nil {
		stats.GlobalInstrumentation.LogEvent("context cancellation")
		return nil
	}

	if result.Depth >= m.maxDepth {
		return nil
	}

	parentURL := result.URL
	if result.RedirectURL != "" {
		parentURL = result.RedirectURL
	}

	if m.adaptiveEngine != nil {
		parsed, err := url.Parse(parentURL)
		if err == nil {
			scheme := "http"
			if parsed.Scheme != "" {
				scheme = parsed.Scheme
			}
			hostRoot := fmt.Sprintf("%s://%s", scheme, parsed.Host)
			newJobs := m.adaptiveEngine.CheckRuntimeTech(hostRoot, parsed.Host)
			if len(newJobs) > 0 {
				var jobsToPush []engine.Job
				for _, j := range newJobs {
					key := normalizeURL(j.URL)
					if _, seen := visited[key]; !seen {
						visited[key] = struct{}{}
						m.HighPriorityCount++
						j.Depth = result.Depth + 1
						jobsToPush = append(jobsToPush, j)
					}
				}
				if len(jobsToPush) > 0 {
					frontier.PushFront(NewSliceGenerator(jobsToPush))
				}
			}
		}
	}

	if result.Depth > 0 && m.recurseOn.Match(result.StatusCode) {
		if m.strategy == DFS {
			m.DFSCount++
		} else {
			m.BFSCount++
		}
	}

	// Process HTML-extracted links (same-host only) as high-priority jobs
	if len(result.Links) > 0 {
		parentParsed, err := url.Parse(parentURL)
		if err == nil {
			var jobs []engine.Job
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
						m.HighPriorityCount++
						jobs = append(jobs, engine.Job{
							URL:    resolvedStr,
							Depth:  result.Depth + 1,
							Origin: engine.OriginHTML,
						})
					}
				}
			}
			if len(jobs) > 0 {
				frontier.PushFront(NewSliceGenerator(jobs))
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

	gen, err := NewDirectoryGenerator(
		ctx,
		m.reader,
		parentURL,
		parentPath,
		int(result.Depth+1),
		parentResContentType,
		prioritizedSegments,
		prioritizedPaths,
		laravel,
		wp,
		express,
		m.normalizePaths,
		m.collapseSlashes,
		m.extensions,
		visited,
		m.fingerprintCache,
		m.stats,
		&m.HighPriorityCount,
		&m.LowPriorityCount,
	)
	if err != nil {
		if m.warningHandler != nil {
			m.warningHandler(fmt.Sprintf("ERROR: failed to load wordlist: %v", err))
		}
		return err
	}

	if m.stats != nil {
		m.stats.RecordDirectoryDiscovered()
		m.stats.RecordDirectoryQueued()
	}

	if m.strategy == DFS || m.strategy == Priority {
		if *activeGenerator != nil {
			frontier.PushFront(*activeGenerator)
		}
		*activeGenerator = gen
	} else {
		frontier.PushBack(gen)
	}
	return nil
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

// FormatRecurseWarning constructs the user-facing warning message when
// the root HTTP response does not satisfy the current recursion policy.
func FormatRecurseWarning(statusCode int, policyStr string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[!] The root URL returned HTTP %d.\r\n\r\n", statusCode))
	sb.WriteString(fmt.Sprintf("    The current recursion policy (--recurse-on: %s)\r\n", policyStr))
	sb.WriteString(fmt.Sprintf("    does not recurse into HTTP %d responses.\r\n\r\n", statusCode))
	sb.WriteString("    No recursive scan was performed.\r\n\r\n")
	sb.WriteString("    If this is intentional, include ")
	sb.WriteString(fmt.Sprintf("%d in the recursion policy, for example:\r\n\r\n", statusCode))
	sb.WriteString(fmt.Sprintf("        --recurse-on %s,%d\r\n\r\n", policyStr, statusCode))
	sb.WriteString("    or\r\n\r\n")
	sb.WriteString(fmt.Sprintf("        --recurse-on %d\r\n\r\n", statusCode))
	sb.WriteString("    Also verify that the target URL is correct.")
	return sb.String()
}
