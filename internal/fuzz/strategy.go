package fuzz

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unsubble/searchit/internal/adaptive"
	"github.com/unsubble/searchit/internal/adaptive/summary"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/stats"
	"golang.org/x/time/rate"
)

// Executor manages a concurrent worker pool for executing jobs.
//
// Single-Owner Channel Invariant:
// Executor enforces strict single-owner channel semantics for jobsChan.
// Producers submit jobs via ExecuteAsync(). Close() MUST only be invoked
// after all producer goroutines have terminated and joined (e.g. via sync.WaitGroup
// or channel drainage). No mutex is required because ExecuteAsync() and close(jobsChan)
// can never run concurrently.
type Executor struct {
	ctx          context.Context
	client       *http.Client
	fs           *filter.FilterSuite
	workers      int
	delay        time.Duration
	limiter      *rate.Limiter
	collector    *stats.Collector
	jobsChan     chan WorkItem
	resultsChan  <-chan Result
	PauseBlocker func(context.Context) error
	closeOnce    sync.Once
}

// NewExecutor initializes and starts the worker pool.
func NewExecutor(
	ctx context.Context,
	drainCtx context.Context,
	client *http.Client,
	fs *filter.FilterSuite,
	workers int,
	delay time.Duration,
	limiter *rate.Limiter,
	collector *stats.Collector,
	pauseBlocker func(context.Context) error,
) *Executor {
	bufSize := workers * 32
	if bufSize < 512 {
		bufSize = 512
	}
	jobsChan := make(chan WorkItem, bufSize)
	resultsChan := Start(ctx, drainCtx, client, fs, workers, delay, limiter, jobsChan, collector, pauseBlocker)

	e := &Executor{
		ctx:          ctx,
		client:       client,
		fs:           fs,
		workers:      workers,
		delay:        delay,
		limiter:      limiter,
		collector:    collector,
		jobsChan:     jobsChan,
		resultsChan:  resultsChan,
		PauseBlocker: pauseBlocker,
	}

	return e
}

// ExecuteAsync schedules a job and returns a channel that will receive the result.
func (e *Executor) ExecuteAsync(job RequestDTO) (<-chan Result, error) {
	if e.PauseBlocker != nil {
		if err := e.PauseBlocker(e.ctx); err != nil {
			return nil, err
		}
	}

	ch := make(chan Result, 1)
	item := WorkItem{
		Req:   job,
		Reply: ch,
	}

	select {
	case <-e.ctx.Done():
		return nil, e.ctx.Err()
	case e.jobsChan <- item:
	}

	atomic.AddInt64(&stats.GlobalInstrumentation.JobsProduced, 1)
	atomic.AddInt64(&stats.GlobalInstrumentation.JobsSubmitted, 1)
	if e.collector != nil {
		e.collector.RecordJobProduced()
	}

	return ch, nil
}

// Execute schedules a job and blocks until its result is received.
func (e *Executor) Execute(job RequestDTO) (Result, error) {
	ch, err := e.ExecuteAsync(job)
	if err != nil {
		return Result{}, err
	}

	select {
	case <-e.ctx.Done():
		return Result{}, e.ctx.Err()
	case res := <-ch:
		return res, nil
	}
}

// Close signals worker pool termination by closing jobsChan.
//
// Invariant: Callers must guarantee that all producer goroutines invoking ExecuteAsync()
// have exited and joined before calling Close().
func (e *Executor) Close() {
	e.closeOnce.Do(func() {
		close(e.jobsChan)
	})
}

// ResultCallback is invoked when a successful result is found.
type ResultCallback func(Result)

// Runner manages the strategy execution.
type Runner struct {
	TargetURL       string
	Method          string
	BodyTemplate    string
	HeaderTemplates http.Header
	CookieTemplate  string

	FuzzWords []string
	FooWords  []string
	BarWords  []string
	BazWords  []string
	BuzzWords []string

	Client    *http.Client
	FS        *filter.FilterSuite
	Threads   int
	Delay     time.Duration
	Limiter   *rate.Limiter
	Collector *stats.Collector

	Quiet       bool
	ShowHeaders bool
	ShowTitle   bool

	Adaptive       bool
	AdaptiveEngine *adaptive.Engine
	Cache          *fingerprint.Cache
	Summary        *summary.Summary

	PauseBlocker func(context.Context) error
	InfoHandler  func(string) // optional hook for informational messages (progress-renderer-aware)

	// StreamingMode is true when the primary wordlist is not pre-counted.
	// When true, pushCandidate calls AddTotalCandidates(1) per job so that
	// totalWork grows incrementally during the scan.
	// When false (pre-counted wordlist), SetTotalCandidates was already called
	// at startup; AddTotalCandidates must NOT be called to avoid double-counting.
	StreamingMode bool

	compiledReq *compiledRequest
}

// printInfo emits an informational message via InfoHandler when set,
// or falls back to writing directly to os.Stderr.
func (r *Runner) printInfo(msg string) {
	if r.InfoHandler != nil {
		r.InfoHandler(msg)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
}

// EstimateCandidates calculates the theoretical maximum search space based on
// the number of primary words, secondary placeholders, and the target template.
func (r *Runner) EstimateCandidates(primaryWordlistSize int) int64 {
	req := RequestTemplate{
		URL:     r.TargetURL,
		Method:  r.Method,
		Body:    r.BodyTemplate,
		Headers: r.HeaderTemplates,
		Cookie:  r.CookieTemplate,
	}
	placeholders := FindPlaceholders(req)

	hasFUZZ := false
	for _, p := range placeholders {
		if p == "FUZZ" {
			hasFUZZ = true
			break
		}
	}

	if primaryWordlistSize <= 0 && len(r.FuzzWords) > 0 {
		primaryWordlistSize = len(r.FuzzWords)
	}

	total := int64(1)
	if hasFUZZ {
		if primaryWordlistSize > 0 {
			total *= int64(primaryWordlistSize)
		} else {
			return 0
		}
	}

	if len(r.FooWords) > 0 {
		total *= int64(len(r.FooWords))
	}
	if len(r.BarWords) > 0 {
		total *= int64(len(r.BarWords))
	}
	if len(r.BazWords) > 0 {
		total *= int64(len(r.BazWords))
	}
	if len(r.BuzzWords) > 0 {
		total *= int64(len(r.BuzzWords))
	}

	return total
}

type compiledHeader struct {
	key    CompiledTemplate
	values []CompiledTemplate
}

type compiledRequest struct {
	targetURL             CompiledTemplate
	body                  CompiledTemplate
	headers               []compiledHeader
	fuzzedHeaders         []compiledHeader
	cookie                CompiledTemplate
	hasHeaderPlaceholders bool
	hasCookiePlaceholders bool
	hasBodyPlaceholders   bool
	isJSONBody            bool
}

func (r *Runner) compileRequest() *compiledRequest {
	bodyTmpl := CompileTemplate(r.BodyTemplate, SupportedPlaceholders)
	cookieTmpl := CompileTemplate(r.CookieTemplate, SupportedPlaceholders)
	cr := &compiledRequest{
		targetURL:             CompileTemplate(r.TargetURL, SupportedPlaceholders),
		body:                  bodyTmpl,
		cookie:                cookieTmpl,
		hasBodyPlaceholders:   bodyTmpl.HasPlaceholders(),
		hasCookiePlaceholders: cookieTmpl.HasPlaceholders(),
		isJSONBody:            isJSONString(r.BodyTemplate),
	}

	var keys []string
	for k := range r.HeaderTemplates {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		vals := r.HeaderTemplates[k]
		ck := CompileTemplate(k, SupportedPlaceholders)
		cvals := make([]CompiledTemplate, 0, len(vals))
		hasHPh := ck.HasPlaceholders()
		for _, v := range vals {
			cv := CompileTemplate(v, SupportedPlaceholders)
			if cv.HasPlaceholders() {
				hasHPh = true
			}
			cvals = append(cvals, cv)
		}
		ch := compiledHeader{key: ck, values: cvals}
		cr.headers = append(cr.headers, ch)
		if hasHPh {
			cr.fuzzedHeaders = append(cr.fuzzedHeaders, ch)
			cr.hasHeaderPlaceholders = true
		}
	}
	return cr
}

// PrepareTemplates compiles the request templates and caches them in the Runner.
// It must be called before BuildJob or IterateCandidates when not going through Run().
// Run() calls this automatically; call it explicitly only from the dry-run path.
func (r *Runner) PrepareTemplates() {
	if r.compiledReq == nil {
		r.compiledReq = r.compileRequest()
	}
}

// CompiledURLTemplate returns the pre-compiled URL template.
// PrepareTemplates must be called first.
func (r *Runner) CompiledURLTemplate() CompiledTemplate {
	return r.compiledReq.targetURL
}

// Run executes the fuzzer according to selected strategy.
func (r *Runner) Run(ctx context.Context, drainCtx context.Context, strategy string, primaryChan <-chan string, yield ResultCallback) error {
	r.compiledReq = r.compileRequest()
	e := NewExecutor(ctx, drainCtx, r.Client, r.FS, r.Threads, r.Delay, r.Limiter, r.Collector, r.PauseBlocker)
	defer e.Close()

	if r.Collector != nil {
		r.Collector.SetIsFinite(true)
	}

	if r.Adaptive {
		if r.AdaptiveEngine == nil {
			r.AdaptiveEngine = adaptive.NewEngine(r.TargetURL, r.Client, r.Cache, r.Quiet)
		}
		r.Summary = r.AdaptiveEngine.Summary
		_ = r.AdaptiveEngine.Discover(ctx)
		// Drain the primary wordlist channel into FuzzWords before building the
		// traversal plan, so the full candidate list is available to adaptive
		// scoring. DFS/BFS/Priority do this too; adaptive mode previously skipped it,
		// causing buildTraversalPlan() to see FuzzWords==nil and fall back to
		// words=[""] — yielding exactly one candidate regardless of wordlist size.
		if primaryChan != nil {
			for word := range primaryChan {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				r.FuzzWords = append(r.FuzzWords, word)
			}
		}
		if r.Collector != nil && len(r.FuzzWords) > 0 {
			r.Collector.SetTotalCandidates(r.EstimateCandidates(len(r.FuzzWords)))
		}
	}

	plan := r.buildTraversalPlan()
	if len(plan.Levels) == 0 {
		return r.runEager(ctx, e, primaryChan, yield)
	}

	if r.Adaptive {
		return r.runAdaptive(ctx, e, plan, yield)
	}

	stratLower := strings.ToLower(strategy)
	if stratLower == "dfs" || stratLower == "bfs" || stratLower == "priority" {
		if primaryChan != nil {
			var mat []string
			for word := range primaryChan {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				mat = append(mat, word)
			}
			r.FuzzWords = mat
			if r.Collector != nil && len(r.FuzzWords) > 0 {
				r.Collector.SetTotalCandidates(r.EstimateCandidates(len(r.FuzzWords)))
			}
			plan = r.buildTraversalPlan()
		}
		if stratLower == "dfs" {
			return r.runDFS(ctx, e, plan, yield)
		}
		if stratLower == "priority" {
			return r.runPriority(ctx, e, plan, yield)
		}
		return r.runBFS(ctx, e, plan, yield)
	}

	return r.runEager(ctx, e, primaryChan, yield)
}

// IterateCandidates walks the full Cartesian product of the primary wordlist
// with the secondary placeholder lists (FOO, BAR, BAZ, BUZZ) and calls yield
// for each variable map. The map key order is always FUZZ/FOO/BAR/BAZ/BUZZ.
// yield must return true to continue or false to abort early.
// This is the single source of truth for candidate ordering; both runEager and
// the dry-run path must call this function to guarantee candidate #N parity.
func (r *Runner) IterateCandidates(ctx context.Context, primaryChan <-chan string, yield func(vars map[string]string) bool) {
	fooList := r.FooWords
	if len(fooList) == 0 {
		fooList = []string{""}
	}
	barList := r.BarWords
	if len(barList) == 0 {
		barList = []string{""}
	}
	bazList := r.BazWords
	if len(bazList) == 0 {
		bazList = []string{""}
	}
	buzzList := r.BuzzWords
	if len(buzzList) == 0 {
		buzzList = []string{""}
	}

	expand := func(fuzzVal string) bool {
		for _, fooVal := range fooList {
			for _, barVal := range barList {
				for _, bazVal := range bazList {
					for _, buzzVal := range buzzList {
						vars := map[string]string{
							"FUZZ": fuzzVal,
							"FOO":  fooVal,
							"BAR":  barVal,
							"BAZ":  bazVal,
							"BUZZ": buzzVal,
						}
						if !yield(vars) {
							return false
						}
					}
				}
			}
		}
		return true
	}

	if primaryChan != nil {
		for word := range primaryChan {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !expand(word) {
				return
			}
		}
	} else {
		fuzzList := r.FuzzWords
		if len(fuzzList) == 0 {
			// Fallback for legacy single-placeholder unit tests that set FooWords for FUZZ
			hasFOO := strings.Contains(r.TargetURL, "FOO") || strings.Contains(r.BodyTemplate, "FOO") || strings.Contains(r.CookieTemplate, "FOO")
			if !hasFOO && len(r.FooWords) > 0 {
				fuzzList = r.FooWords
			} else {
				fuzzList = []string{""}
			}
		}
		for _, fuzzVal := range fuzzList {
			if !expand(fuzzVal) {
				return
			}
		}
	}
}

func (r *Runner) runEager(ctx context.Context, e *Executor, primaryChan <-chan string, yield ResultCallback) error {
	bufSize := r.Threads * 32
	if bufSize < 512 {
		bufSize = 512
	}
	jobChan := make(chan (<-chan Result), bufSize)
	var producerWg sync.WaitGroup
	producerWg.Add(1)

	go func() {
		defer producerWg.Done()
		defer close(jobChan)

		r.IterateCandidates(ctx, primaryChan, func(vars map[string]string) bool {
			job, err := r.BuildJob(r.compiledReq.targetURL, vars)
			if err != nil {
				if r.Collector != nil {
					r.Collector.RecordSkipped(1)
				}
				return true
			}
			asyncCh, err := e.ExecuteAsync(job)
			if err != nil {
				return false
			}
			select {
			case <-ctx.Done():
				return false
			case jobChan <- asyncCh:
				return true
			}
		})
	}()

	for resCh := range jobChan {
		select {
		case <-ctx.Done():
			producerWg.Wait()
			return ctx.Err()
		case res := <-resCh:
			if res.Accepted || res.Err != nil {
				yield(res)
			}
		}
	}

	producerWg.Wait()
	return nil
}

// BuildJob renders a single RequestDTO from a compiled URL template and a map of
// placeholder → value substitutions.
// This is exported so that the dry-run path can produce RequestDTOs using the
// exact same rendering logic as the live execution path, guaranteeing candidate
// #N parity between dry-run and real runs for the same configuration.
func (r *Runner) BuildJob(urlTemplate CompiledTemplate, vars map[string]string) (RequestDTO, error) {
	urlStr := urlTemplate.RenderString(vars)

	var bodyStr string
	if len(r.compiledReq.body) > 0 {
		bodyStr = r.compiledReq.body.RenderString(vars)
	}

	headers := make(map[string][]string)
	for _, ch := range r.compiledReq.headers {
		newK := ch.key.RenderString(vars)
		var newValues []string
		for _, val := range ch.values {
			newValues = append(newValues, val.RenderString(vars))
		}
		headers[newK] = newValues
	}

	var cookies []string
	if len(r.compiledReq.cookie) > 0 {
		cookies = append(cookies, r.compiledReq.cookie.RenderString(vars))
	}

	var fuzzData *engine.FuzzData
	if r.compiledReq.hasHeaderPlaceholders || r.compiledReq.hasCookiePlaceholders || r.compiledReq.hasBodyPlaceholders {
		var fields []engine.FuzzField
		if r.compiledReq.hasHeaderPlaceholders {
			for _, ch := range r.compiledReq.fuzzedHeaders {
				newK := ch.key.RenderString(vars)
				for _, val := range ch.values {
					newV := val.RenderString(vars)
					fields = append(fields, engine.FuzzField{
						Location: engine.LocationHeader,
						Name:     newK,
						Value:    newV,
					})
				}
			}
		}
		if r.compiledReq.hasCookiePlaceholders && len(cookies) > 0 {
			for _, cStr := range cookies {
				if cStr != "" {
					name, val := parseCookieField(cStr)
					fields = append(fields, engine.FuzzField{
						Location: engine.LocationCookie,
						Name:     name,
						Value:    val,
					})
				}
			}
		}
		if r.compiledReq.hasBodyPlaceholders && bodyStr != "" {
			loc := engine.LocationBody
			if r.compiledReq.isJSONBody {
				loc = engine.LocationJSON
			}
			fields = append(fields, engine.FuzzField{
				Location: loc,
				Value:    bodyStr,
			})
		}
		if len(fields) > 0 {
			fuzzData = &engine.FuzzData{Fields: fields}
		}
	}

	return RequestDTO{
		URL:      urlStr,
		Method:   r.Method,
		Body:     bodyStr,
		Headers:  headers,
		Cookies:  cookies,
		FuzzData: fuzzData,
	}, nil
}
