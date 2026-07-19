package fuzz

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unsubble/searchit/internal/adaptive"
	"github.com/unsubble/searchit/internal/adaptive/types"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/stats"
	"golang.org/x/time/rate"
)

// Executor manages a concurrent worker pool for executing jobs.
type Executor struct {
	ctx         context.Context
	client      *http.Client
	fs          *filter.FilterSuite
	workers     int
	delay       time.Duration
	limiter     *rate.Limiter
	collector   *stats.Collector
	jobsChan    chan Job
	resultsChan <-chan Result

	mu      sync.Mutex
	pending map[uint64]chan Result
	nextID  uint64
}

// NewExecutor initializes and starts the worker pool.
func NewExecutor(
	ctx context.Context,
	client *http.Client,
	fs *filter.FilterSuite,
	workers int,
	delay time.Duration,
	limiter *rate.Limiter,
	collector *stats.Collector,
) *Executor {
	jobsChan := make(chan Job, workers*2)
	resultsChan := Start(ctx, client, fs, workers, delay, limiter, jobsChan, collector)

	e := &Executor{
		ctx:         ctx,
		client:      client,
		fs:          fs,
		workers:     workers,
		delay:       delay,
		limiter:     limiter,
		collector:   collector,
		jobsChan:    jobsChan,
		resultsChan: resultsChan,
		pending:     make(map[uint64]chan Result),
	}

	go e.routeResults()

	return e
}

func (e *Executor) routeResults() {
	for res := range e.resultsChan {
		id, ok := res.UserData.(uint64)
		if !ok {
			continue
		}
		e.mu.Lock()
		ch, found := e.pending[id]
		if found {
			delete(e.pending, id)
		}
		e.mu.Unlock()
		if found {
			ch <- res
			close(ch)
		}
	}
}

// Execute schedules a job and blocks until its result is received.
func (e *Executor) Execute(job Job) (Result, error) {
	e.mu.Lock()
	id := e.nextID
	e.nextID++
	ch := make(chan Result, 1)
	e.pending[id] = ch
	e.mu.Unlock()

	job.UserData = id

	select {
	case <-e.ctx.Done():
		return Result{}, e.ctx.Err()
	case e.jobsChan <- job:
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
	if depth == 1 {
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
	} else if depth == 2 {
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
func (r *Runner) Run(ctx context.Context, strategy string, primaryChan <-chan string, yield ResultCallback) error {
	e := NewExecutor(ctx, r.Client, r.FS, r.Threads, r.Delay, r.Limiter, r.Collector)
	defer e.Close()

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

type eagerJobInfo struct {
	foo  string
	bar  string
	buzz string
	fuzz string
	idx  int
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

	if primaryChan != nil {
		var currentBatch []string
		batchSize := r.Threads * 4
		if batchSize < 32 {
			batchSize = 32
		}
		for {
			select {
			case <-ctx.Done():
				return nil
			case word, ok := <-primaryChan:
				if !ok {
					if len(currentBatch) > 0 {
						r.executeEagerBatch(ctx, e, currentBatch, fooList, barList, buzzList, yield)
					}
					return nil
				}
				currentBatch = append(currentBatch, word)
				if len(currentBatch) >= batchSize {
					r.executeEagerBatch(ctx, e, currentBatch, fooList, barList, buzzList, yield)
					currentBatch = nil
				}
			}
		}
	} else {
		var allJobs []eagerJobInfo
		idx := 0
		for _, fooVal := range fooList {
			for _, barVal := range barList {
				for _, buzzVal := range buzzList {
					allJobs = append(allJobs, eagerJobInfo{
						foo:  fooVal,
						bar:  barVal,
						buzz: buzzVal,
						idx:  idx,
					})
					idx++
				}
			}
		}

		batchSize := r.Threads * 4
		if batchSize < 32 {
			batchSize = 32
		}

		for start := 0; start < len(allJobs); start += batchSize {
			end := start + batchSize
			if end > len(allJobs) {
				end = len(allJobs)
			}

			batch := allJobs[start:end]
			batchResults := make([]Result, len(batch))
			var wg sync.WaitGroup

			for i, info := range batch {
				wg.Add(1)
				go func(localIdx int, jobInfo eagerJobInfo) {
					defer wg.Done()
					job := r.buildJob(r.TargetURL, jobInfo.foo, jobInfo.bar, jobInfo.buzz)
					res, err := e.Execute(job)
					if err == nil {
						batchResults[localIdx] = res
					}
				}(i, info)
			}
			wg.Wait()

			for _, res := range batchResults {
				if res.Accepted {
					yield(res)
				}
			}
		}
	}

	return nil
}

func (r *Runner) executeEagerBatch(
	ctx context.Context,
	e *Executor,
	fuzzVals []string,
	fooList, barList, buzzList []string,
	yield ResultCallback,
) {
	var batch []eagerJobInfo
	idx := 0
	for _, fuzzVal := range fuzzVals {
		for _, fooVal := range fooList {
			for _, barVal := range barList {
				for _, buzzVal := range buzzList {
					batch = append(batch, eagerJobInfo{
						foo:  fooVal,
						bar:  barVal,
						buzz: buzzVal,
						fuzz: fuzzVal,
						idx:  idx,
					})
					idx++
				}
			}
		}
	}

	batchResults := make([]Result, len(batch))
	var wg sync.WaitGroup

	for i, info := range batch {
		wg.Add(1)
		go func(localIdx int, jobInfo eagerJobInfo) {
			defer wg.Done()
			job := r.buildJobWithFuzz(r.TargetURL, jobInfo.fuzz, jobInfo.foo, jobInfo.bar, jobInfo.buzz)
			res, err := e.Execute(job)
			if err == nil {
				batchResults[localIdx] = res
			}
		}(i, info)
	}
	wg.Wait()

	for _, res := range batchResults {
		if res.Accepted {
			yield(res)
		}
	}
}

func (r *Runner) runBFS(ctx context.Context, e *Executor, yield ResultCallback) error {
	maxDepth := GetTargetDepth(r.TargetURL)
	if maxDepth == 0 {
		return r.runEager(ctx, e, nil, yield)
	}

	// Level 1: Fuzz FOO
	tmpl1 := TruncateTemplate(r.TargetURL, 1)
	var foundFOO []string

	type jobResult struct {
		word string
		res  Result
	}
	results1 := make([]jobResult, len(r.FooWords))
	var wg sync.WaitGroup

	for i, word := range r.FooWords {
		wg.Add(1)
		go func(idx int, w string) {
			defer wg.Done()
			job := r.buildJob(tmpl1, w, "", "")
			res, err := e.Execute(job)
			if err == nil {
				results1[idx] = jobResult{word: w, res: res}
			}
		}(i, word)
	}
	wg.Wait()

	for _, jr := range results1 {
		if jr.res.Accepted {
			yield(jr.res)
			foundFOO = append(foundFOO, jr.word)
		}
	}

	if len(foundFOO) == 0 || maxDepth < 2 {
		return nil
	}

	// Level 2: Fuzz BAR
	tmpl2 := TruncateTemplate(r.TargetURL, 2)
	var foundBAR []struct {
		foo string
		bar string
	}

	results2 := make([]jobResult, len(foundFOO)*len(r.BarWords))
	jobIdx := 0
	type barJob struct {
		foo string
		bar string
		idx int
	}
	var barJobs []barJob

	for _, fooVal := range foundFOO {
		for _, barVal := range r.BarWords {
			barJobs = append(barJobs, barJob{foo: fooVal, bar: barVal, idx: jobIdx})
			jobIdx++
		}
	}

	for _, bj := range barJobs {
		wg.Add(1)
		go func(info barJob) {
			defer wg.Done()
			job := r.buildJob(tmpl2, info.foo, info.bar, "")
			res, err := e.Execute(job)
			if err == nil {
				results2[info.idx] = jobResult{word: info.foo + "/" + info.bar, res: res}
			}
		}(bj)
	}
	wg.Wait()

	for _, bj := range barJobs {
		jr := results2[bj.idx]
		if jr.res.Accepted {
			yield(jr.res)
			foundBAR = append(foundBAR, struct{ foo, bar string }{bj.foo, bj.bar})
		}
	}

	if len(foundBAR) == 0 || maxDepth < 3 {
		return nil
	}

	// Level 3: Fuzz BUZZ
	results3 := make([]jobResult, len(foundBAR)*len(r.BuzzWords))
	jobIdx = 0
	type buzzJob struct {
		foo  string
		bar  string
		buzz string
		idx  int
	}
	var buzzJobs []buzzJob

	for _, parent := range foundBAR {
		for _, buzzVal := range r.BuzzWords {
			buzzJobs = append(buzzJobs, buzzJob{foo: parent.foo, bar: parent.bar, buzz: buzzVal, idx: jobIdx})
			jobIdx++
		}
	}

	for _, bj := range buzzJobs {
		wg.Add(1)
		go func(info buzzJob) {
			defer wg.Done()
			job := r.buildJob(r.TargetURL, info.foo, info.bar, info.buzz)
			res, err := e.Execute(job)
			if err == nil {
				results3[info.idx] = jobResult{word: info.foo + "/" + info.bar + "/" + info.buzz, res: res}
			}
		}(bj)
	}
	wg.Wait()

	for _, bj := range buzzJobs {
		jr := results3[bj.idx]
		if jr.res.Accepted {
			yield(jr.res)
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

		if currentDepth == 1 {
			tmpl := TruncateTemplate(r.TargetURL, 1)
			results := make([]Result, len(r.FooWords))
			var wg sync.WaitGroup
			for i, word := range r.FooWords {
				wg.Add(1)
				go func(idx int, w string) {
					defer wg.Done()
					job := r.buildJob(tmpl, w, "", "")
					res, err := e.Execute(job)
					if err == nil {
						results[idx] = res
					}
				}(i, word)
			}
			wg.Wait()

			for i, res := range results {
				if res.Accepted {
					yield(res)
					if maxDepth >= 2 {
						dfsVisit(r.FooWords[i], "", 2)
					}
				}
			}
		} else if currentDepth == 2 {
			tmpl := TruncateTemplate(r.TargetURL, 2)
			results := make([]Result, len(r.BarWords))
			var wg sync.WaitGroup
			for i, word := range r.BarWords {
				wg.Add(1)
				go func(idx int, w string) {
					defer wg.Done()
					job := r.buildJob(tmpl, parentFoo, w, "")
					res, err := e.Execute(job)
					if err == nil {
						results[idx] = res
					}
				}(i, word)
			}
			wg.Wait()

			for i, res := range results {
				if res.Accepted {
					yield(res)
					if maxDepth >= 3 {
						dfsVisit(parentFoo, r.BarWords[i], 3)
					}
				}
			}
		} else if currentDepth == 3 {
			results := make([]Result, len(r.BuzzWords))
			var wg sync.WaitGroup
			for i, word := range r.BuzzWords {
				wg.Add(1)
				go func(idx int, w string) {
					defer wg.Done()
					job := r.buildJob(r.TargetURL, parentFoo, parentBar, w)
					res, err := e.Execute(job)
					if err == nil {
						results[idx] = res
					}
				}(i, word)
			}
			wg.Wait()

			for _, res := range results {
				if res.Accepted {
					yield(res)
				}
			}
		}
	}

	dfsVisit("", "", 1)
	return nil
}

func (r *Runner) runAdaptive(ctx context.Context, e *Executor, yield ResultCallback) error {
	engine := adaptive.NewEngine(r.TargetURL, r.Client, r.Cache, r.Quiet)
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
			dots := strings.Repeat(".", 15-len(item.word))
			if len(dots) < 3 {
				dots = "..."
			}
			fmt.Printf("    %s %s %d\n", item.word, dots, item.score)
		}
		fmt.Fprint(os.Stdout, "\nTraversal decisions:\n\n")
	}

	tmpl1 := TruncateTemplate(r.TargetURL, 1)

	sortedFoo := make([]string, len(r.FooWords))
	copy(sortedFoo, r.FooWords)
	sort.SliceStable(sortedFoo, func(i, j int) bool {
		return engine.GetScore(sortedFoo[i], nil, 1, "") > engine.GetScore(sortedFoo[j], nil, 1, "")
	})

	sortedIndices := make(map[string]int)
	for i, w := range sortedFoo {
		sortedIndices[w] = i
	}

	results1 := make([]Result, len(r.FooWords))
	var wg sync.WaitGroup
	for _, word := range sortedFoo {
		wg.Add(1)
		go func(w string) {
			defer wg.Done()
			job := r.buildJob(tmpl1, w, "", "")
			res, err := e.Execute(job)
			if err == nil {
				results1[sortedIndices[w]] = res
			}
		}(word)
	}
	wg.Wait()

	originalFooIndices := make(map[string]int)
	for i, w := range r.FooWords {
		originalFooIndices[w] = i
	}

	type orderedResult struct {
		res   Result
		index int
	}
	var orderedRes1 []orderedResult
	for _, w := range sortedFoo {
		res := results1[sortedIndices[w]]
		if res.Accepted {
			orderedRes1 = append(orderedRes1, orderedResult{res: res, index: originalFooIndices[w]})
		}
	}
	sort.Slice(orderedRes1, func(i, j int) bool {
		return orderedRes1[i].index < orderedRes1[j].index
	})

	type branchDecision struct {
		foo        string
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
				dots := strings.Repeat(".", 12-len(fooVal))
				if len(dots) < 3 {
					dots = "..."
				}
				fmt.Printf("    /%s %s %s (rule: %s)\n", fooVal, dots, dec.Policy, dec.Rule)
			}

			decisions = append(decisions, branchDecision{
				foo:        fooVal,
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
			engine.Summary.RecordFindings(len(orderedRes1))
			engine.Summary.Print(os.Stdout)
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
							job := r.buildJob(tmpl2, d.foo, bVal, "")
							res, err := e.Execute(job)
							if err == nil {
								results[i] = res
							}
						}(idx, barVal)
					}
					eagerWg.Wait()

					for barIdx, res := range results {
						if res.Accepted {
							arMutex.Lock()
							allResults = append(allResults, adaptiveResult{
								res:     res,
								fooIdx:  originalFooIndices[d.foo],
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
							job := r.buildJob(r.TargetURL, d.foo, info.bar, info.buzz)
							res, err := e.Execute(job)
							if err == nil {
								results[i] = res
							}
						}(idx, jobInfo)
					}
					eagerWg.Wait()

					for idx, res := range results {
						if res.Accepted {
							arMutex.Lock()
							allResults = append(allResults, adaptiveResult{
								res:     res,
								fooIdx:  originalFooIndices[d.foo],
								barIdx:  jobs[idx].barIdx,
								buzzIdx: jobs[idx].buzzIdx,
								depth:   3,
							})
							arMutex.Unlock()
						}
					}
				}
				return
			}

			// BFS or DFS policies
			sortedBar := make([]string, len(r.BarWords))
			copy(sortedBar, r.BarWords)
			sort.SliceStable(sortedBar, func(i, j int) bool {
				ct := d.res.Headers.Get("Content-Type")
				return engine.GetScore(sortedBar[i], []string{d.foo}, 2, ct) > engine.GetScore(sortedBar[j], []string{d.foo}, 2, ct)
			})

			sortedBarIndices := make(map[string]int)
			for i, w := range sortedBar {
				sortedBarIndices[w] = i
			}

			barResults := make([]Result, len(r.BarWords))
			var innerWg sync.WaitGroup
			for _, barVal := range sortedBar {
				innerWg.Add(1)
				go func(bVal string) {
					defer innerWg.Done()
					job := r.buildJob(tmpl2, d.foo, bVal, "")
					res, err := e.Execute(job)
					if err == nil {
						barResults[sortedBarIndices[bVal]] = res
					}
				}(barVal)
			}
			innerWg.Wait()

			originalBarIndices := make(map[string]int)
			for i, w := range r.BarWords {
				originalBarIndices[w] = i
			}

			for _, barVal := range sortedBar {
				res := barResults[sortedBarIndices[barVal]]
				if res.Accepted {
					fooIdx := originalFooIndices[d.foo]
					barIdx := originalBarIndices[barVal]

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
						sortedBuzz := make([]string, len(r.BuzzWords))
						copy(sortedBuzz, r.BuzzWords)
						sort.SliceStable(sortedBuzz, func(i, j int) bool {
							ct := res.Headers.Get("Content-Type")
							return engine.GetScore(sortedBuzz[i], []string{d.foo, barVal}, 3, ct) > engine.GetScore(sortedBuzz[j], []string{d.foo, barVal}, 3, ct)
						})

						sortedBuzzIndices := make(map[string]int)
						for i, w := range sortedBuzz {
							sortedBuzzIndices[w] = i
						}

						buzzResults := make([]Result, len(r.BuzzWords))
						var leafWg sync.WaitGroup
						for _, buzzVal := range sortedBuzz {
							leafWg.Add(1)
							go func(bzVal string) {
								defer leafWg.Done()
								job := r.buildJob(r.TargetURL, d.foo, barVal, bzVal)
								r3, err := e.Execute(job)
								if err == nil {
									buzzResults[sortedBuzzIndices[bzVal]] = r3
								}
							}(buzzVal)
						}
						leafWg.Wait()

						originalBuzzIndices := make(map[string]int)
						for i, w := range r.BuzzWords {
							originalBuzzIndices[w] = i
						}

						for _, buzzVal := range sortedBuzz {
							r3 := buzzResults[sortedBuzzIndices[buzzVal]]
							if r3.Accepted {
								buzzIdx := originalBuzzIndices[buzzVal]
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

					sortedBuzz := make([]string, len(r.BuzzWords))
					copy(sortedBuzz, r.BuzzWords)
					sort.SliceStable(sortedBuzz, func(i, j int) bool {
						ct := n.res.Headers.Get("Content-Type")
						return engine.GetScore(sortedBuzz[i], []string{fooVal, barVal}, 3, ct) > engine.GetScore(sortedBuzz[j], []string{fooVal, barVal}, 3, ct)
					})

					sortedBuzzIndices := make(map[string]int)
					for i, w := range sortedBuzz {
						sortedBuzzIndices[w] = i
					}

					buzzResults := make([]Result, len(r.BuzzWords))
					var leafWg sync.WaitGroup
					for _, buzzVal := range sortedBuzz {
						leafWg.Add(1)
						go func(bzVal string) {
							defer leafWg.Done()
							job := r.buildJob(r.TargetURL, fooVal, barVal, bzVal)
							r3, err := e.Execute(job)
							if err == nil {
								buzzResults[sortedBuzzIndices[bzVal]] = r3
							}
						}(buzzVal)
					}
					leafWg.Wait()

					originalBuzzIndices := make(map[string]int)
					for i, w := range r.BuzzWords {
						originalBuzzIndices[w] = i
					}

					for _, buzzVal := range sortedBuzz {
						r3 := buzzResults[sortedBuzzIndices[buzzVal]]
						if r3.Accepted {
							buzzIdx := originalBuzzIndices[buzzVal]
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
		engine.Summary.RecordFindings(len(allResults))
		engine.Summary.Print(os.Stdout)
	}

	return nil
}

type scoredWord struct {
	word  string
	index int
	score int
}

func sortWordsByPriority(
	words []string,
	parentPath []string,
	depth int,
	r *Runner,
	parentRes *Result,
	prioritizedSegments, prioritizedFullPaths map[string]bool,
) []string {
	scored := make([]scoredWord, len(words))
	for i, w := range words {
		score := getPriorityScore(w, parentPath, depth, r, parentRes, prioritizedSegments, prioritizedFullPaths)
		scored[i] = scoredWord{word: w, index: i, score: score}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	out := make([]string, len(words))
	for i, sw := range scored {
		out[i] = sw.word
	}
	return out
}

func getPriorityScore(
	word string,
	parentPath []string,
	depth int,
	r *Runner,
	parentRes *Result,
	prioritizedSegments, prioritizedFullPaths map[string]bool,
) int {
	score := 0

	if prioritizedSegments[strings.ToLower(word)] {
		score += 5
	}

	fullPath := word
	if len(parentPath) > 0 {
		fullPath = strings.Join(parentPath, "/") + "/" + word
	}
	if prioritizedFullPaths[strings.ToLower(fullPath)] {
		score += 10
	}

	if parentRes != nil && parentRes.Headers != nil {
		ct := parentRes.Headers.Get("Content-Type")
		if strings.Contains(ct, "json") || strings.Contains(ct, "xml") || strings.Contains(ct, "javascript") {
			apiKeywords := map[string]bool{
				"api": true, "v1": true, "v2": true, "v3": true, "graphql": true,
				"json": true, "rest": true, "users": true, "login": true, "auth": true,
				"admin": true, "config": true, "status": true, "test": true, "dev": true,
			}
			if apiKeywords[strings.ToLower(word)] {
				score += 3
			}
		} else if strings.Contains(ct, "html") {
			htmlKeywords := map[string]bool{
				"index": true, "home": true, "about": true, "contact": true, "login": true,
				"register": true, "dashboard": true, "admin": true, "blog": true, "pages": true,
			}
			if htmlKeywords[strings.ToLower(word)] {
				score += 2
			}
		}
	}

	if r.Adaptive && r.Cache != nil {
		u, err := url.Parse(r.TargetURL)
		if err == nil {
			if fp := r.Cache.Get(u.Host); fp != nil {
				for _, sig := range fp.Signals() {
					val := strings.ToLower(sig.Value)
					src := strings.ToLower(sig.Source)
					if strings.Contains(val, "php") || strings.Contains(src, "php") {
						if strings.Contains(strings.ToLower(word), "php") {
							score += 5
						}
					}
					if strings.Contains(val, "wordpress") || strings.Contains(src, "wordpress") {
						if strings.Contains(strings.ToLower(word), "wp-") || strings.Contains(strings.ToLower(word), "wordpress") {
							score += 8
						}
					}
					if strings.Contains(val, "laravel") || strings.Contains(src, "laravel") {
						if strings.Contains(strings.ToLower(word), "artisan") || strings.Contains(strings.ToLower(word), "storage") || strings.Contains(strings.ToLower(word), "nova") {
							score += 8
						}
					}
				}
			}
		}
	}

	return score
}

func (r *Runner) buildJob(tmpl, fooVal, barVal, buzzVal string) Job {
	urlStr := r.replacePlaceholders(tmpl, "", fooVal, barVal, buzzVal)

	var bodyBytes []byte
	if r.BodyTemplate != "" {
		bodyStr := r.replacePlaceholders(r.BodyTemplate, "", fooVal, barVal, buzzVal)
		bodyBytes = []byte(bodyStr)
	}

	headers := make(http.Header)
	for k, values := range r.HeaderTemplates {
		newK := r.replacePlaceholders(k, "", fooVal, barVal, buzzVal)
		var newValues []string
		for _, val := range values {
			newValues = append(newValues, r.replacePlaceholders(val, "", fooVal, barVal, buzzVal))
		}
		headers[newK] = newValues
	}

	var cookies []*http.Cookie
	if r.CookieTemplate != "" {
		cookieStr := r.replacePlaceholders(r.CookieTemplate, "", fooVal, barVal, buzzVal)
		cookies = parseCookies(cookieStr)
	}

	return Job{
		URL:     urlStr,
		Method:  r.Method,
		Body:    bodyBytes,
		Headers: headers,
		Cookies: cookies,
	}
}

func (r *Runner) buildJobWithFuzz(tmpl, fuzzVal, fooVal, barVal, buzzVal string) Job {
	urlStr := r.replacePlaceholders(tmpl, fuzzVal, fooVal, barVal, buzzVal)

	var bodyBytes []byte
	if r.BodyTemplate != "" {
		bodyStr := r.replacePlaceholders(r.BodyTemplate, fuzzVal, fooVal, barVal, buzzVal)
		bodyBytes = []byte(bodyStr)
	}

	headers := make(http.Header)
	for k, values := range r.HeaderTemplates {
		newK := r.replacePlaceholders(k, fuzzVal, fooVal, barVal, buzzVal)
		var newValues []string
		for _, val := range values {
			newValues = append(newValues, r.replacePlaceholders(val, fuzzVal, fooVal, barVal, buzzVal))
		}
		headers[newK] = newValues
	}

	var cookies []*http.Cookie
	if r.CookieTemplate != "" {
		cookieStr := r.replacePlaceholders(r.CookieTemplate, fuzzVal, fooVal, barVal, buzzVal)
		cookies = parseCookies(cookieStr)
	}

	return Job{
		URL:     urlStr,
		Method:  r.Method,
		Body:    bodyBytes,
		Headers: headers,
		Cookies: cookies,
	}
}

func (r *Runner) replacePlaceholders(template, fuzzVal, fooVal, barVal, buzzVal string) string {
	res := template
	res = strings.ReplaceAll(res, "FUZZ", fuzzVal)
	res = strings.ReplaceAll(res, "FOO", fooVal)
	res = strings.ReplaceAll(res, "BAR", barVal)
	res = strings.ReplaceAll(res, "BUZZ", buzzVal)
	return res
}
