package fuzz

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unsubble/searchit/internal/adaptive/summary"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/stats"
	"golang.org/x/time/rate"
)

// Executor manages a concurrent worker pool for executing jobs.
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

// Close signals worker pool termination.
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

	Adaptive bool
	Cache    *fingerprint.Cache
	Summary  *summary.Summary

	PauseBlocker func(context.Context) error

	compiledReq *compiledRequest
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

	total := int64(1)
	if hasFUZZ && primaryWordlistSize > 0 {
		total *= int64(primaryWordlistSize)
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

// Run executes the fuzzer according to selected strategy.
func (r *Runner) Run(ctx context.Context, drainCtx context.Context, strategy string, primaryChan <-chan string, yield ResultCallback) error {
	r.compiledReq = r.compileRequest()
	e := NewExecutor(ctx, drainCtx, r.Client, r.FS, r.Threads, r.Delay, r.Limiter, r.Collector, r.PauseBlocker)
	defer e.Close()

	if r.Collector != nil {
		r.Collector.SetIsFinite(true)
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
			r.FooWords = mat
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

func (r *Runner) runEager(ctx context.Context, e *Executor, primaryChan <-chan string, yield ResultCallback) error {
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

	bufSize := r.Threads * 32
	if bufSize < 512 {
		bufSize = 512
	}
	jobChan := make(chan (<-chan Result), bufSize)

	go func() {
		defer close(jobChan)

		pushCandidate := func(vars map[string]string) bool {
			job, err := r.buildJob(r.compiledReq.targetURL, vars)
			if err != nil {
				if r.Collector != nil {
					r.Collector.RecordSkipped(1)
				}
				return true
			}
			atomic.AddInt64(&stats.GlobalInstrumentation.JobsProduced, 1)
			atomic.AddInt64(&stats.GlobalInstrumentation.JobsSubmitted, 1)
			if e.collector != nil {
				e.collector.RecordJobProduced()
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
		}

		fuzzList := r.FooWords
		if primaryChan != nil {
			fuzzList = nil
		}

		if primaryChan != nil {
			for word := range primaryChan {
				select {
				case <-ctx.Done():
					return
				default:
				}
				vars := map[string]string{
					"FUZZ": word,
					"FOO":  word,
				}
				for _, barVal := range barList {
					vars["BAR"] = barVal
					for _, bazVal := range bazList {
						vars["BAZ"] = bazVal
						for _, buzzVal := range buzzList {
							vars["BUZZ"] = buzzVal
							if !pushCandidate(vars) {
								return
							}
						}
					}
				}
			}
		} else {
			if len(fuzzList) == 0 {
				fuzzList = []string{""}
			}
			for _, fuzzVal := range fuzzList {
				vars := map[string]string{
					"FUZZ": fuzzVal,
					"FOO":  fuzzVal,
				}
				for _, barVal := range barList {
					vars["BAR"] = barVal
					for _, bazVal := range bazList {
						vars["BAZ"] = bazVal
						for _, buzzVal := range buzzList {
							vars["BUZZ"] = buzzVal
							if !pushCandidate(vars) {
								return
							}
						}
					}
				}
			}
		}
	}()

	for resCh := range jobChan {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res := <-resCh:
			if res.Accepted || res.Err != nil {
				yield(res)
			}
		}
	}

	return nil
}

func (r *Runner) buildJob(urlTemplate CompiledTemplate, vars map[string]string) (RequestDTO, error) {
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
