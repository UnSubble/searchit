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
	"github.com/unsubble/searchit/internal/adaptive/types"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/presentation"
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
	jobsChan := make(chan WorkItem, workers*2)
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
	close(e.jobsChan)
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

// GetTargetDepth checks placeholder levels in target URL template.
func GetTargetDepth(urlTemplate string) int {
	hasFOO := strings.Contains(urlTemplate, "FOO")
	hasBAR := strings.Contains(urlTemplate, "BAR")
	hasBUZZ := strings.Contains(urlTemplate, "BUZZ")

	if hasBUZZ && hasBAR && hasFOO {
		return 3
	}
	if hasBAR && hasFOO {
		return 2
	}
	if hasFOO {
		return 1
	}
	return 0
}

// TruncateTemplate cuts template segments for a specific target depth.
func TruncateTemplate(urlTemplate string, depth int) string {
	switch depth {
	case 1:
		if idx := strings.Index(urlTemplate, "/BAR"); idx != -1 {
			return urlTemplate[:idx]
		}
		if idx := strings.Index(urlTemplate, "BAR"); idx != -1 {
			return urlTemplate[:idx]
		}
		if idx := strings.Index(urlTemplate, "/BUZZ"); idx != -1 {
			return urlTemplate[:idx]
		}
		if idx := strings.Index(urlTemplate, "BUZZ"); idx != -1 {
			return urlTemplate[:idx]
		}
	case 2:
		if idx := strings.Index(urlTemplate, "/BUZZ"); idx != -1 {
			return urlTemplate[:idx]
		}
		if idx := strings.Index(urlTemplate, "BUZZ"); idx != -1 {
			return urlTemplate[:idx]
		}
	}
	return urlTemplate
}

// Run executes the fuzzer according to selected strategy.
func (r *Runner) Run(ctx context.Context, drainCtx context.Context, strategy string, primaryChan <-chan string, yield ResultCallback) error {
	r.compiledReq = r.compileRequest()
	e := NewExecutor(ctx, drainCtx, r.Client, r.FS, r.Threads, r.Delay, r.Limiter, r.Collector, r.PauseBlocker)
	defer e.Close()

	if r.Collector != nil {
		r.Collector.SetIsFinite(true)
	}

	maxDepth := GetTargetDepth(r.TargetURL)
	if maxDepth == 0 {
		return r.runEager(ctx, e, primaryChan, yield)
	}

	if r.Adaptive {
		return r.runAdaptive(ctx, e, yield)
	}

	switch strings.ToLower(strategy) {
	case "bfs":
		return r.runBFS(ctx, e, yield)
	case "dfs":
		return r.runDFS(ctx, e, yield)
	case "eager":
		return r.runEager(ctx, e, primaryChan, yield)
	default:
		return r.runEager(ctx, e, primaryChan, yield)
	}
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
	buzzList := r.BuzzWords
	if len(buzzList) == 0 {
		buzzList = []string{""}
	}

	batchSize := r.Threads * 4
	if batchSize < 32 {
		batchSize = 32
	}

	jobChan := make(chan chan Result, batchSize)

	// Declare WaitGroup first so it is pushed onto the defer stack FIRST.
	// This ensures it executes LAST during stack unwinding.
	var producerWg sync.WaitGroup
	defer producerWg.Wait()

	// Declare context cancellation next so it is pushed onto the defer stack SECOND.
	// This ensures it executes FIRST during stack unwinding.
	producerCtx, cancelProducer := context.WithCancel(ctx)
	defer cancelProducer()

	producerWg.Add(1)
	go func() {
		defer producerWg.Done()
		defer close(jobChan)

		if primaryChan != nil {
			for {
				select {
				case <-producerCtx.Done():
					return
				case word, ok := <-primaryChan:
					if !ok {
						return
					}
					for _, fooVal := range fooList {
						for _, barVal := range barList {
							for _, buzzVal := range buzzList {
								resCh := make(chan Result, 1)
								select {
								case <-producerCtx.Done():
									return
								case jobChan <- resCh:
								}
								producerWg.Add(1)
								go func(f, foo, bar, buzz string, ch chan<- Result) {
									defer producerWg.Done()
									job, err := r.buildJob(r.compiledReq.targetURL, map[string]string{"FUZZ": f, "FOO": foo, "BAR": bar, "BUZZ": buzz})
									if err != nil {
										if r.Collector != nil {
											r.Collector.RecordSkipped(1)
										}
										ch <- Result{Err: err}
										return
									}
									res, err := e.Execute(job)
									if err != nil {
										res.Err = err
									}
									ch <- res
								}(word, fooVal, barVal, buzzVal, resCh)
							}
						}
					}
				}
			}
		} else {
			for _, fooVal := range fooList {
				for _, barVal := range barList {
					for _, buzzVal := range buzzList {
						resCh := make(chan Result, 1)
						select {
						case <-producerCtx.Done():
							return
						case jobChan <- resCh:
						}
						producerWg.Add(1)
						go func(foo, bar, buzz string, ch chan<- Result) {
							defer producerWg.Done()
							job, err := r.buildJob(r.compiledReq.targetURL, map[string]string{"FOO": foo, "BAR": bar, "BUZZ": buzz})
							if err != nil {
								if r.Collector != nil {
									r.Collector.RecordSkipped(1)
								}
								ch <- Result{Err: err}
								return
							}
							res, err := e.Execute(job)
							if err != nil {
								res.Err = err
							}
							ch <- res
						}(fooVal, barVal, buzzVal, resCh)
					}
				}
			}
		}
	}()

	for resCh := range jobChan {
		// Always drain resCh, regardless of ctx state.
		// If we skip the receive after selecting ctx.Done(), the background goroutine
		// that owns resCh blocks trying to send, which fills jobChan and deadlocks
		// the producer goroutine — causing workers to appear stuck.
		res := <-resCh
		if ctx.Err() == nil {
			if res.Accepted || res.Err != nil {
				yield(res)
			}
		}
	}

	return nil
}

func (r *Runner) runBFS(ctx context.Context, e *Executor, yield ResultCallback) error {
	maxDepth := GetTargetDepth(r.TargetURL)
	if maxDepth == 0 {
		return r.runEager(ctx, e, nil, yield)
	}

	// Level 1: Fuzz FOO
	tmpl1 := TruncateTemplate(r.TargetURL, 1)
	cTmpl1 := CompileTemplate(tmpl1, SupportedPlaceholders)
	var foundFOO []string

	type pendingJob1 struct {
		word string
		ch   <-chan Result
		err  error
	}
	pending1 := make(chan pendingJob1, 1024)

	go func() {
		defer close(pending1)
		for _, word := range r.FooWords {
			select {
			case <-ctx.Done():
				return
			default:
			}
			vars := map[string]string{"FOO": word}
			job, err := r.buildJob(cTmpl1, vars)
			if err != nil {
				pending1 <- pendingJob1{word: word, err: err}
				continue
			}
			if maxDepth >= 2 {
				job.IsProbing = true
			}
			asyncCh, err := e.ExecuteAsync(job)
			pending1 <- pendingJob1{word: word, ch: asyncCh, err: err}
		}
	}()

	for p := range pending1 {
		if p.err != nil {
			if r.Collector != nil {
				pruned := int64(1)
				if maxDepth >= 2 {
					pruned *= int64(len(r.BarWords))
				}
				if maxDepth >= 3 {
					pruned *= int64(len(r.BuzzWords))
				}
				r.Collector.RecordSearchSpaceProgress(pruned)
			}
			continue
		}
		res := <-p.ch

		if (res.Accepted || res.Err != nil) && ctx.Err() == nil {
			yield(res)
		}
		if ctx.Err() != nil {
			continue
		}
		if res.Accepted {
			foundFOO = append(foundFOO, p.word)
		} else if r.Collector != nil {
			var pruned int64
			if maxDepth >= 2 {
				pruned = int64(len(r.BarWords))
				if maxDepth >= 3 {
					pruned *= int64(len(r.BuzzWords))
				}
			}
			if pruned > 0 {
				r.Collector.RecordSkipped(pruned)
			}
		}
	}

	if len(foundFOO) == 0 || maxDepth < 2 {
		return nil
	}

	// Level 2: Fuzz BAR
	tmpl2 := TruncateTemplate(r.TargetURL, 2)
	cTmpl2 := CompileTemplate(tmpl2, SupportedPlaceholders)
	var foundBAR []struct {
		foo string
		bar string
	}

	type barJob struct {
		foo string
		bar string
	}
	var barJobs []barJob

	for _, fooVal := range foundFOO {
		for _, barVal := range r.BarWords {
			barJobs = append(barJobs, barJob{foo: fooVal, bar: barVal})
		}
	}
	type pendingJob2 struct {
		info barJob
		ch   <-chan Result
		err  error
	}
	pending2 := make(chan pendingJob2, 1024)

	go func() {
		defer close(pending2)
		for _, bj := range barJobs {
			select {
			case <-ctx.Done():
				return
			default:
			}
			vars := map[string]string{"FOO": bj.foo, "BAR": bj.bar}
			job, err := r.buildJob(cTmpl2, vars)
			if err != nil {
				pending2 <- pendingJob2{info: bj, err: err}
				continue
			}
			if maxDepth >= 3 {
				job.IsProbing = true
			}
			asyncCh, err := e.ExecuteAsync(job)
			pending2 <- pendingJob2{info: bj, ch: asyncCh, err: err}
		}
	}()

	for p := range pending2 {
		if p.err != nil {
			if r.Collector != nil {
				pruned := int64(1)
				if maxDepth >= 3 {
					pruned *= int64(len(r.BuzzWords))
				}
				r.Collector.RecordSkipped(pruned)
			}
			continue
		}
		res := <-p.ch

		if (res.Accepted || res.Err != nil) && ctx.Err() == nil {
			yield(res)
		}
		if ctx.Err() != nil {
			continue
		}
		if res.Accepted {
			foundBAR = append(foundBAR, struct {
				foo string
				bar string
			}{foo: p.info.foo, bar: p.info.bar})
		} else if r.Collector != nil {
			var pruned int64
			if maxDepth >= 3 {
				pruned = int64(len(r.BuzzWords))
			}
			if pruned > 0 {
				r.Collector.RecordSkipped(pruned)
			}
		}
	}

	if len(foundBAR) == 0 || maxDepth < 3 {
		return nil
	}

	// Level 3: Fuzz BUZZ
	cTmpl3 := CompileTemplate(r.TargetURL, SupportedPlaceholders)
	type buzzJob struct {
		foo  string
		bar  string
		buzz string
	}
	var buzzJobs []buzzJob

	for _, barVal := range foundBAR {
		for _, buzzVal := range r.BuzzWords {
			buzzJobs = append(buzzJobs, buzzJob{foo: barVal.foo, bar: barVal.bar, buzz: buzzVal})
		}
	}

	type pendingJob3 struct {
		info buzzJob
		ch   <-chan Result
		err  error
	}
	pending3 := make(chan pendingJob3, 1024)

	go func() {
		defer close(pending3)
		for _, bj := range buzzJobs {
			select {
			case <-ctx.Done():
				return
			default:
			}
			vars := map[string]string{"FOO": bj.foo, "BAR": bj.bar, "BUZZ": bj.buzz}
			job, err := r.buildJob(cTmpl3, vars)
			if err != nil {
				pending3 <- pendingJob3{info: bj, err: err}
				continue
			}
			asyncCh, err := e.ExecuteAsync(job)
			pending3 <- pendingJob3{info: bj, ch: asyncCh, err: err}
		}
	}()

	for p := range pending3 {
		if p.err != nil {
			if r.Collector != nil {
				r.Collector.RecordSkipped(1)
			}
			continue
		}
		res := <-p.ch

		if (res.Accepted || res.Err != nil) && ctx.Err() == nil {
			yield(res)
		}
	}

	return nil
}

func (r *Runner) runDFS(ctx context.Context, e *Executor, yield ResultCallback) error {
	maxDepth := GetTargetDepth(r.TargetURL)
	if maxDepth == 0 {
		return r.runEager(ctx, e, nil, yield)
	}

	var dfsVisit func(parentFoo, parentBar string, currentDepth int)
	dfsVisit = func(parentFoo, parentBar string, currentDepth int) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		switch currentDepth {
		case 1:
			tmpl := TruncateTemplate(r.TargetURL, 1)
			cTmpl := CompileTemplate(tmpl, SupportedPlaceholders)

			type pendingDFS struct {
				word string
				ch   <-chan Result
				err  error
			}
			pending1 := make(chan pendingDFS, 1024)

			go func() {
				defer close(pending1)
				for _, word := range r.FooWords {
					select {
					case <-ctx.Done():
						return
					default:
					}
					vars := map[string]string{"FOO": word}
					job, err := r.buildJob(cTmpl, vars)
					if err != nil {
						pending1 <- pendingDFS{word: word, err: err}
						continue
					}
					if maxDepth >= 2 {
						job.IsProbing = true
					}
					asyncCh, err := e.ExecuteAsync(job)
					pending1 <- pendingDFS{word: word, ch: asyncCh, err: err}
				}
			}()

			for p := range pending1 {
				if p.err != nil {
					if r.Collector != nil {
						pruned := int64(1)
						if maxDepth >= 2 {
							pruned *= int64(len(r.BarWords))
						}
						if maxDepth >= 3 {
							pruned *= int64(len(r.BuzzWords))
						}
						r.Collector.RecordSearchSpaceProgress(pruned)
					}
					continue
				}
				res := <-p.ch

				if ctx.Err() == nil {
					if res.Accepted || res.Err != nil {
						yield(res)
					}
					if !res.Accepted && r.Collector != nil {
						var pruned int64
						if maxDepth >= 2 {
							pruned = int64(len(r.BarWords))
							if maxDepth >= 3 {
								pruned *= int64(len(r.BuzzWords))
							}
						}
						if pruned > 0 {
							r.Collector.RecordSkipped(pruned)
						}
					}
				}
				if ctx.Err() != nil {
					continue
				}

				if res.Accepted {
					if maxDepth >= 2 {
						dfsVisit(p.word, "", 2)
					}
				}
			}
		case 2:
			tmpl := TruncateTemplate(r.TargetURL, 2)
			cTmpl := CompileTemplate(tmpl, SupportedPlaceholders)

			type pendingDFS struct {
				word string
				ch   <-chan Result
				err  error
			}
			pending2 := make(chan pendingDFS, 1024)

			go func() {
				defer close(pending2)
				for _, word := range r.BarWords {
					select {
					case <-ctx.Done():
						return
					default:
					}
					vars := map[string]string{"FOO": parentFoo, "BAR": word}
					job, err := r.buildJob(cTmpl, vars)
					if err != nil {
						pending2 <- pendingDFS{word: word, err: err}
						continue
					}
					if maxDepth >= 3 {
						job.IsProbing = true
					}
					asyncCh, err := e.ExecuteAsync(job)
					pending2 <- pendingDFS{word: word, ch: asyncCh, err: err}
				}
			}()

			for p := range pending2 {
				if p.err != nil {
					if r.Collector != nil {
						var pruned int64 = 1
						if maxDepth >= 3 {
							pruned *= int64(len(r.BuzzWords))
						}
						r.Collector.RecordSkipped(pruned)
					}
					continue
				}
				res := <-p.ch

				if ctx.Err() == nil {
					if res.Accepted || res.Err != nil {
						yield(res)
					}
					if !res.Accepted && r.Collector != nil {
						var pruned int64
						if maxDepth >= 3 {
							pruned = int64(len(r.BuzzWords))
						}
						if pruned > 0 {
							r.Collector.RecordSkipped(pruned)
						}
					}
				}

				if ctx.Err() != nil {
					continue
				}

				if res.Accepted {
					if maxDepth >= 3 {
						dfsVisit(parentFoo, p.word, 3)
					}
				}
			}
		case 3:
			type pendingDFS struct {
				word string
				ch   <-chan Result
				err  error
			}
			pending3 := make(chan pendingDFS, 1024)

			go func() {
				defer close(pending3)
				for _, word := range r.BuzzWords {
					select {
					case <-ctx.Done():
						return
					default:
					}
					vars := map[string]string{"FOO": parentFoo, "BAR": parentBar, "BUZZ": word}
					job, err := r.buildJob(r.compiledReq.targetURL, vars)
					if err != nil {
						pending3 <- pendingDFS{word: word, err: err}
						continue
					}
					asyncCh, err := e.ExecuteAsync(job)
					pending3 <- pendingDFS{word: word, ch: asyncCh, err: err}
				}
			}()

			for p := range pending3 {
				if p.err != nil {
					if r.Collector != nil {
						r.Collector.RecordSkipped(1)
					}
					continue
				}
				res := <-p.ch

				if ctx.Err() == nil {
					if res.Accepted || res.Err != nil {
						yield(res)
					}
				}
			}
		}
	}

	dfsVisit("", "", 1)
	return nil
}

func (r *Runner) runAdaptive(ctx context.Context, e *Executor, yield ResultCallback) error {
	type payload struct {
		word    string
		origIdx int
	}

	engine := adaptive.NewEngine(r.TargetURL, r.Client, r.Cache, r.Quiet)
	r.Summary = engine.Summary
	if err := engine.Discover(ctx); err != nil {
		return err
	}

	if !r.Quiet {
		fmt.Fprint(os.Stdout, "\nPriority scores:\n\n")
		type scoredItem struct {
			word  string
			score int
		}
		var scoredItems []scoredItem
		for _, w := range r.FooWords {
			score := engine.GetScore(w, nil, 1, "")
			scoredItems = append(scoredItems, scoredItem{word: w, score: score})
		}
		sort.Slice(scoredItems, func(i, j int) bool {
			return scoredItems[i].score > scoredItems[j].score
		})
		for _, item := range scoredItems {
			fmt.Fprintf(os.Stderr, "    %-15s %s\n", item.word, presentation.Number(int64(item.score)))
		}
		fmt.Fprint(os.Stderr, "\nTraversal decisions:\n\n")
	}

	tmpl1 := TruncateTemplate(r.TargetURL, 1)
	cTmpl1 := CompileTemplate(tmpl1, SupportedPlaceholders)

	payloads1 := make([]payload, len(r.FooWords))
	for i, w := range r.FooWords {
		payloads1[i] = payload{word: w, origIdx: i}
	}
	sort.SliceStable(payloads1, func(i, j int) bool {
		return engine.GetScore(payloads1[i].word, nil, 1, "") > engine.GetScore(payloads1[j].word, nil, 1, "")
	})

	results1 := make([]Result, len(r.FooWords))
	var wg sync.WaitGroup
	for i, p := range payloads1 {
		wg.Add(1)
		go func(idx int, p payload) {
			defer wg.Done()
			vars := map[string]string{"FOO": p.word}
			job, err := r.buildJob(cTmpl1, vars)
			if err != nil {

				return
			}
			res, err := e.Execute(job)

			if err == nil {
				results1[idx] = res
			}
		}(i, p)
	}
	wg.Wait()

	type orderedResult struct {
		res   Result
		index int
	}
	var orderedRes1 []orderedResult
	for i, p := range payloads1 {
		res := results1[i]
		if (res.Accepted || res.Err != nil) && ctx.Err() == nil {
			orderedRes1 = append(orderedRes1, orderedResult{res: res, index: p.origIdx})
		}
	}
	sort.Slice(orderedRes1, func(i, j int) bool {
		return orderedRes1[i].index < orderedRes1[j].index
	})

	type branchDecision struct {
		foo        string
		fooIdx     int
		res        Result
		policy     types.Policy
		policyRule string
		score      int
	}
	var decisions []branchDecision

	for _, or := range orderedRes1 {
		parts := strings.Split(strings.TrimRight(or.res.URL, "/"), "/")
		if len(parts) > 0 {
			fooVal := parts[len(parts)-1]
			ct := or.res.Headers.Get("Content-Type")
			sigs := engine.GetSignals(fooVal, nil, 1, ct)
			score := engine.GetScore(fooVal, nil, 1, ct)
			dec := engine.SelectTraversal(sigs)

			engine.Summary.RecordTraversal(dec.Policy)

			if !r.Quiet {
				val := fmt.Sprintf("%s (rule: %s)", dec.Policy, dec.Rule)
				fmt.Fprintf(os.Stderr, "    %-12s %s\n", "/"+fooVal, val)
			}

			decisions = append(decisions, branchDecision{
				foo:        fooVal,
				fooIdx:     or.index,
				res:        or.res,
				policy:     dec.Policy,
				policyRule: dec.Rule,
				score:      score,
			})
		}
	}

	maxDepth := GetTargetDepth(r.TargetURL)
	if maxDepth < 2 || len(decisions) == 0 {
		for _, or := range orderedRes1 {
			yield(or.res)
		}
		if !r.Quiet {

		}
		return nil
	}

	type adaptiveResult struct {
		res     Result
		fooIdx  int
		barIdx  int
		buzzIdx int
		depth   int
	}
	var allResults []adaptiveResult
	var arMutex sync.Mutex

	for _, or := range orderedRes1 {
		allResults = append(allResults, adaptiveResult{
			res:     or.res,
			fooIdx:  or.index,
			barIdx:  -1,
			buzzIdx: -1,
			depth:   1,
		})
	}

	tmpl2 := TruncateTemplate(r.TargetURL, 2)
	cTmpl2 := CompileTemplate(tmpl2, SupportedPlaceholders)
	var branchWg sync.WaitGroup

	for _, dec := range decisions {
		branchWg.Add(1)
		go func(d branchDecision) {
			defer branchWg.Done()

			if d.policy == types.PolicyEager {
				// Run Eager cartesian fuzzing under this parent branch
				if maxDepth == 2 {
					var eagerWg sync.WaitGroup
					results := make([]Result, len(r.BarWords))
					for idx, barVal := range r.BarWords {
						eagerWg.Add(1)
						go func(i int, bVal string) {
							defer eagerWg.Done()
							vars := map[string]string{"FOO": d.foo, "BAR": bVal}
							job, err := r.buildJob(cTmpl2, vars)
							if err != nil {

								return
							}
							res, err := e.Execute(job)

							if err == nil {
								results[i] = res
							}
						}(idx, barVal)
					}
					eagerWg.Wait()

					for barIdx, res := range results {
						if (res.Accepted || res.Err != nil) && ctx.Err() == nil {
							arMutex.Lock()
							allResults = append(allResults, adaptiveResult{
								res:     res,
								fooIdx:  d.fooIdx,
								barIdx:  barIdx,
								buzzIdx: -1,
								depth:   2,
							})
							arMutex.Unlock()
						}
					}
				} else if maxDepth >= 3 {
					type eagerJob struct {
						bar     string
						buzz    string
						barIdx  int
						buzzIdx int
					}
					var jobs []eagerJob
					for barIdx, barVal := range r.BarWords {
						for buzzIdx, buzzVal := range r.BuzzWords {
							jobs = append(jobs, eagerJob{
								bar:     barVal,
								buzz:    buzzVal,
								barIdx:  barIdx,
								buzzIdx: buzzIdx,
							})
						}
					}

					results := make([]Result, len(jobs))
					var eagerWg sync.WaitGroup
					for idx, jobInfo := range jobs {
						eagerWg.Add(1)
						go func(i int, info eagerJob) {
							defer eagerWg.Done()
							vars := map[string]string{"FOO": d.foo, "BAR": info.bar, "BUZZ": info.buzz}
							job, err := r.buildJob(r.compiledReq.targetURL, vars)
							if err != nil {

								return
							}
							res, err := e.Execute(job)

							if err == nil {
								results[i] = res
							}
						}(idx, jobInfo)
					}
					eagerWg.Wait()

					for i, res := range results {
						if (res.Accepted || res.Err != nil) && ctx.Err() == nil {
							arMutex.Lock()
							allResults = append(allResults, adaptiveResult{
								res:     res,
								fooIdx:  d.fooIdx,
								barIdx:  jobs[i].barIdx,
								buzzIdx: jobs[i].buzzIdx,
								depth:   3,
							})
							arMutex.Unlock()
						}
					}
				}
				return
			}

			// BFS or DFS policies
			payloads2 := make([]payload, len(r.BarWords))
			for i, w := range r.BarWords {
				payloads2[i] = payload{word: w, origIdx: i}
			}
			sort.SliceStable(payloads2, func(i, j int) bool {
				ct := d.res.Headers.Get("Content-Type")
				return engine.GetScore(payloads2[i].word, []string{d.foo}, 2, ct) > engine.GetScore(payloads2[j].word, []string{d.foo}, 2, ct)
			})

			barResults := make([]Result, len(r.BarWords))
			var innerWg sync.WaitGroup
			for i, p := range payloads2 {
				innerWg.Add(1)
				go func(idx int, p payload) {
					defer innerWg.Done()
					vars := map[string]string{"FOO": d.foo, "BAR": p.word}
					job, err := r.buildJob(cTmpl2, vars)
					if err != nil {

						return
					}
					res, err := e.Execute(job)

					if err == nil {
						barResults[idx] = res
					}
				}(i, p)
			}
			innerWg.Wait()

			for i, p := range payloads2 {
				res := barResults[i]
				barVal := p.word
				if (res.Accepted || res.Err != nil) && ctx.Err() == nil {
					fooIdx := d.fooIdx
					barIdx := p.origIdx

					arMutex.Lock()
					allResults = append(allResults, adaptiveResult{
						res:     res,
						fooIdx:  fooIdx,
						barIdx:  barIdx,
						buzzIdx: -1,
						depth:   2,
					})
					arMutex.Unlock()

					if d.policy == types.PolicyDFS && maxDepth >= 3 {
						payloads3 := make([]payload, len(r.BuzzWords))
						for i, w := range r.BuzzWords {
							payloads3[i] = payload{word: w, origIdx: i}
						}
						sort.SliceStable(payloads3, func(i, j int) bool {
							ct := res.Headers.Get("Content-Type")
							return engine.GetScore(payloads3[i].word, []string{d.foo, barVal}, 3, ct) > engine.GetScore(payloads3[j].word, []string{d.foo, barVal}, 3, ct)
						})

						buzzResults := make([]Result, len(r.BuzzWords))
						var leafWg sync.WaitGroup
						for i, p := range payloads3 {
							leafWg.Add(1)
							go func(idx int, p payload) {
								defer leafWg.Done()
								vars := map[string]string{"FOO": d.foo, "BAR": barVal, "BUZZ": p.word}
								job, err := r.buildJob(r.compiledReq.targetURL, vars)
								if err != nil {

									return
								}
								r3, err := e.Execute(job)

								if err == nil {
									buzzResults[idx] = r3
								}
							}(i, p)
						}
						leafWg.Wait()

						for i, p := range payloads3 {
							r3 := buzzResults[i]
							if r3.Accepted {
								buzzIdx := p.origIdx
								arMutex.Lock()
								allResults = append(allResults, adaptiveResult{
									res:     r3,
									fooIdx:  fooIdx,
									barIdx:  barIdx,
									buzzIdx: buzzIdx,
									depth:   3,
								})
								arMutex.Unlock()
							}
						}
					}
				}
			}
		}(dec)
	}
	branchWg.Wait()

	if maxDepth >= 3 {
		var bfsLevel2Nodes []adaptiveResult
		arMutex.Lock()
		for _, ar := range allResults {
			if ar.depth == 2 {
				parentFoo := r.FooWords[ar.fooIdx]
				var policy types.Policy
				for _, dec := range decisions {
					if dec.foo == parentFoo {
						policy = dec.policy
						break
					}
				}
				if policy == types.PolicyBFS {
					bfsLevel2Nodes = append(bfsLevel2Nodes, ar)
				}
			}
		}
		arMutex.Unlock()

		if len(bfsLevel2Nodes) > 0 {
			var bfs3Wg sync.WaitGroup
			for _, node := range bfsLevel2Nodes {
				bfs3Wg.Add(1)
				go func(n adaptiveResult) {
					defer bfs3Wg.Done()

					fooVal := r.FooWords[n.fooIdx]
					barVal := r.BarWords[n.barIdx]

					payloads3 := make([]payload, len(r.BuzzWords))
					for i, w := range r.BuzzWords {
						payloads3[i] = payload{word: w, origIdx: i}
					}
					sort.SliceStable(payloads3, func(i, j int) bool {
						ct := n.res.Headers.Get("Content-Type")
						return engine.GetScore(payloads3[i].word, []string{fooVal, barVal}, 3, ct) > engine.GetScore(payloads3[j].word, []string{fooVal, barVal}, 3, ct)
					})

					buzzResults := make([]Result, len(r.BuzzWords))
					var leafWg sync.WaitGroup
					for i, p := range payloads3 {
						leafWg.Add(1)
						go func(idx int, p payload) {
							defer leafWg.Done()
							vars := map[string]string{"FOO": fooVal, "BAR": barVal, "BUZZ": p.word}
							job, err := r.buildJob(r.compiledReq.targetURL, vars)
							if err != nil {

								return
							}
							r3, err := e.Execute(job)

							if err == nil {
								buzzResults[idx] = r3
							}
						}(i, p)
					}
					leafWg.Wait()

					for i, p := range payloads3 {
						r3 := buzzResults[i]
						if r3.Accepted {
							buzzIdx := p.origIdx
							arMutex.Lock()
							allResults = append(allResults, adaptiveResult{
								res:     r3,
								fooIdx:  n.fooIdx,
								barIdx:  n.barIdx,
								buzzIdx: buzzIdx,
								depth:   3,
							})
							arMutex.Unlock()
						}
					}
				}(node)
			}
			bfs3Wg.Wait()
		}
	}

	sort.SliceStable(allResults, func(i, j int) bool {
		A := allResults[i]
		B := allResults[j]

		if A.fooIdx != B.fooIdx {
			return A.fooIdx < B.fooIdx
		}

		parentFoo := r.FooWords[A.fooIdx]
		var policy types.Policy
		for _, dec := range decisions {
			if dec.foo == parentFoo {
				policy = dec.policy
				break
			}
		}

		if policy == types.PolicyDFS {
			if A.barIdx != B.barIdx {
				if A.barIdx == -1 {
					return true
				}
				if B.barIdx == -1 {
					return false
				}
				return A.barIdx < B.barIdx
			}
			if A.buzzIdx != B.buzzIdx {
				if A.buzzIdx == -1 {
					return true
				}
				if B.buzzIdx == -1 {
					return false
				}
				return A.buzzIdx < B.buzzIdx
			}
			return false
		} else {
			if A.depth != B.depth {
				return A.depth < B.depth
			}
			if A.barIdx != B.barIdx {
				return A.barIdx < B.barIdx
			}
			return A.buzzIdx < B.buzzIdx
		}
	})

	for _, ar := range allResults {
		yield(ar.res)
	}

	if !r.Quiet {

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
