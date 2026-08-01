package fuzz

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unsubble/searchit/internal/adaptive/summary"
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

type compiledRequest struct {
	targetURL CompiledTemplate
	body      CompiledTemplate
	headers   map[*CompiledTemplate][]CompiledTemplate
	cookie    CompiledTemplate
}

func (r *Runner) compileRequest() *compiledRequest {
	cr := &compiledRequest{
		targetURL: CompileTemplate(r.TargetURL, SupportedPlaceholders),
		body:      CompileTemplate(r.BodyTemplate, SupportedPlaceholders),
		cookie:    CompileTemplate(r.CookieTemplate, SupportedPlaceholders),
		headers:   make(map[*CompiledTemplate][]CompiledTemplate),
	}
	for k, vals := range r.HeaderTemplates {
		ck := CompileTemplate(k, SupportedPlaceholders)
		cvals := make([]CompiledTemplate, 0, len(vals))
		for _, v := range vals {
			cvals = append(cvals, CompileTemplate(v, SupportedPlaceholders))
		}
		cr.headers[&ck] = cvals
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
	if stratLower == "dfs" || stratLower == "bfs" {
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
		return r.runBFS(ctx, e, plan, yield)
	}

	return r.runEager(ctx, e, primaryChan, yield)
}

func (r *Runner) runEager(ctx context.Context, e *Executor, primaryChan <-chan string, yield ResultCallback) error {
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

	bufSize := r.Threads * 32
	if bufSize < 512 {
		bufSize = 512
	}
	jobChan := make(chan chan Result, bufSize)

	go func() {
		defer close(jobChan)

		pushCandidate := func(vars map[string]string) bool {
			if e.PauseBlocker != nil {
				if err := e.PauseBlocker(ctx); err != nil {
					return false
				}
			}
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
			resCh := make(chan Result, 1)
			item := WorkItem{
				Req:   job,
				Reply: resCh,
			}
			select {
			case <-ctx.Done():
				return false
			case e.jobsChan <- item:
			}
			select {
			case <-ctx.Done():
				return false
			case jobChan <- resCh:
				return true
			}
		}

		if primaryChan != nil {
			for word := range primaryChan {
				select {
				case <-ctx.Done():
					return
				default:
				}
				for _, fooVal := range fooList {
					for _, barVal := range barList {
						for _, bazVal := range bazList {
							for _, buzzVal := range buzzList {
								if !pushCandidate(map[string]string{
									"FUZZ": word,
									"FOO":  fooVal,
									"BAR":  barVal,
									"BAZ":  bazVal,
									"BUZZ": buzzVal,
								}) {
									return
								}
							}
						}
					}
				}
			}
		} else {
			for _, fooVal := range fooList {
				for _, barVal := range barList {
					for _, bazVal := range bazList {
						for _, buzzVal := range buzzList {
							if !pushCandidate(map[string]string{
								"FOO":  fooVal,
								"BAR":  barVal,
								"BAZ":  bazVal,
								"BUZZ": buzzVal,
							}) {
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
			// Context cancelled. The worker might have dropped this job,
			// so we skip reading from resCh to prevent a deadlock.
			// We continue the loop to drain jobChan until the producer closes it,
			// ensuring the producer has exited before we return.
			continue
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
	for k, vals := range r.compiledReq.headers {
		newK := k.RenderString(vars)
		var newValues []string
		for _, val := range vals {
			newValues = append(newValues, val.RenderString(vars))
		}
		headers[newK] = newValues
	}

	var cookies []string
	if len(r.compiledReq.cookie) > 0 {
		// Split raw cookie strings into distinct Cookie headers if needed,
		// but since Cookie header is usually one line or multiple Cookie headers,
		// we just append the rendered template as a single Cookie string
		cookies = append(cookies, r.compiledReq.cookie.RenderString(vars))
	}

	return RequestDTO{
		URL:     urlStr,
		Method:  r.Method,
		Body:    bodyStr,
		Headers: headers,
		Cookies: cookies,
	}, nil
}
