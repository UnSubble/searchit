package fuzz

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/unsubble/searchit/internal/adaptive"
	"github.com/unsubble/searchit/internal/adaptive/types"
	"github.com/unsubble/searchit/internal/presentation"
)

type TraversalLevel struct {
	Placeholder string
	Words       []string
}

type TraversalPlan struct {
	Levels             []TraversalLevel
	ActivePlaceholders []string
}

type pendingJob struct {
	word string
	idx  int
	vars map[string]string
	ch   <-chan Result
	err  error
}

type evaluateReq struct {
	word string
	idx  int
	vars map[string]string
}

// pendingBuffer defines the capacity for the producer's pending queue.
// This decouples producer submission from consumer processing.
const pendingBuffer = 64

func (r *Runner) evaluateLevel(ctx context.Context, e *Executor, cTmpl CompiledTemplate, isProbing bool, reqs []evaluateReq) <-chan pendingJob {
	pending := make(chan pendingJob, pendingBuffer)

	// Architectural Invariant:
	// The goroutine exists to prevent traversal scheduling from stalling when
	// ExecuteAsync() applies executor backpressure. It ensures the consumer loop
	// can immediately read completed futures and process them (e.g. recursing into
	// deep paths) without waiting for horizontal submission to finish.
	// Future refactors must preserve this property.
	go func() {
		defer close(pending)
		for _, req := range reqs {
			select {
			case <-ctx.Done():
				return
			default:
			}

			job, err := r.buildJob(cTmpl, req.vars)
			if err != nil {
				pending <- pendingJob{word: req.word, idx: req.idx, vars: req.vars, err: err}
				continue
			}
			job.IsProbing = isProbing
			asyncCh, err := e.ExecuteAsync(job)
			pending <- pendingJob{word: req.word, idx: req.idx, vars: req.vars, ch: asyncCh, err: err}
		}
	}()
	return pending
}

func (r *Runner) recordPruned(plan TraversalPlan, currentDepth int) {
	if r.Collector == nil {
		return
	}
	pruned := int64(1)
	for i := currentDepth + 1; i < len(plan.Levels); i++ {
		pruned *= int64(len(plan.Levels[i].Words))
	}
	if pruned > 0 {
		r.Collector.RecordSkipped(pruned)
	}
}

// buildTraversalPlan creates an explicit traversal plan based on the parsed template.
func (r *Runner) buildTraversalPlan() TraversalPlan {
	req := RequestTemplate{
		URL:     r.TargetURL,
		Method:  r.Method,
		Body:    r.BodyTemplate,
		Headers: r.HeaderTemplates,
		Cookie:  r.CookieTemplate,
	}
	activePlaceholders := FindPlaceholders(req)

	var plan TraversalPlan
	plan.ActivePlaceholders = activePlaceholders

	for _, ph := range activePlaceholders {
		switch ph {
		case "FUZZ":
			words := r.FuzzWords
			if len(words) == 0 {
				hasFOO := false
				for _, ap := range activePlaceholders {
					if ap == "FOO" {
						hasFOO = true
						break
					}
				}
				if !hasFOO && len(r.FooWords) > 0 {
					words = r.FooWords
				} else {
					words = []string{""}
				}
			}
			plan.Levels = append(plan.Levels, TraversalLevel{Placeholder: ph, Words: words})
		case "FOO":
			words := r.FooWords
			if len(words) == 0 {
				words = []string{""}
			}
			plan.Levels = append(plan.Levels, TraversalLevel{Placeholder: ph, Words: words})
		case "BAR":
			words := r.BarWords
			if len(words) == 0 {
				words = []string{""}
			}
			plan.Levels = append(plan.Levels, TraversalLevel{Placeholder: ph, Words: words})
		case "BAZ":
			words := r.BazWords
			if len(words) == 0 {
				words = []string{""}
			}
			plan.Levels = append(plan.Levels, TraversalLevel{Placeholder: ph, Words: words})
		case "BUZZ":
			words := r.BuzzWords
			if len(words) == 0 {
				words = []string{""}
			}
			plan.Levels = append(plan.Levels, TraversalLevel{Placeholder: ph, Words: words})
		}
	}
	return plan
}

// TruncateTemplate cuts template segments for a specific target depth.
func (p *TraversalPlan) TruncateTemplate(urlTemplate string, currentDepth int) string {
	if currentDepth >= len(p.Levels)-1 {
		return urlTemplate
	}

	earliestIdx := len(urlTemplate)
	// Truncate at the NEXT level's placeholder, or ANY placeholder after it.
	for i := currentDepth + 1; i < len(p.Levels); i++ {
		ph := p.Levels[i].Placeholder
		phWithSlash := "/" + ph
		if idx := strings.Index(urlTemplate, phWithSlash); idx != -1 && idx < earliestIdx {
			earliestIdx = idx
		} else if idx := strings.Index(urlTemplate, ph); idx != -1 && idx < earliestIdx {
			earliestIdx = idx
		}

		// If FOO and FUZZ are both active, we should truncate on either.
		if ph == "FOO" || ph == "FUZZ" {
			other := "FOO"
			if ph == "FOO" {
				other = "FUZZ"
			}
			otherWithSlash := "/" + other
			if idx := strings.Index(urlTemplate, otherWithSlash); idx != -1 && idx < earliestIdx {
				earliestIdx = idx
			} else if idx := strings.Index(urlTemplate, other); idx != -1 && idx < earliestIdx {
				earliestIdx = idx
			}
		}
	}

	return urlTemplate[:earliestIdx]
}

func (r *Runner) runDFS(ctx context.Context, e *Executor, plan TraversalPlan, yield ResultCallback) error {
	var dfsVisit func(currentDepth int, vars map[string]string)

	dfsVisit = func(currentDepth int, vars map[string]string) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		tmpl := plan.TruncateTemplate(r.TargetURL, currentDepth)
		cTmpl := CompileTemplate(tmpl, SupportedPlaceholders)
		level := plan.Levels[currentDepth]

		var reqs []evaluateReq
		for _, word := range level.Words {
			newVars := make(map[string]string)
			for k, v := range vars {
				newVars[k] = v
			}
			newVars[level.Placeholder] = word
			reqs = append(reqs, evaluateReq{word: word, vars: newVars})
		}

		pending := r.evaluateLevel(ctx, e, cTmpl, currentDepth < len(plan.Levels)-1, reqs)

		for p := range pending {
			if p.err != nil {
				r.recordPruned(plan, currentDepth)
				continue
			}
			res := <-p.ch

			if ctx.Err() == nil {
				if res.Accepted || res.Err != nil {
					yield(res)
				}
				if !res.Accepted && currentDepth < len(plan.Levels)-1 {
					r.recordPruned(plan, currentDepth)
				}
			}
			if ctx.Err() != nil {
				continue
			}

			if res.Accepted {
				if currentDepth < len(plan.Levels)-1 {
					newVars := make(map[string]string)
					for k, v := range vars {
						newVars[k] = v
					}
					newVars[level.Placeholder] = p.word
					dfsVisit(currentDepth+1, newVars)
				}
			}
		}
	}

	dfsVisit(0, make(map[string]string))
	return nil
}

func (r *Runner) runBFS(ctx context.Context, e *Executor, plan TraversalPlan, yield ResultCallback) error {
	type queueItem struct {
		vars map[string]string
	}

	queue := []queueItem{{vars: make(map[string]string)}}

	for depth := 0; depth < len(plan.Levels); depth++ {
		if len(queue) == 0 {
			break
		}

		tmpl := plan.TruncateTemplate(r.TargetURL, depth)
		cTmpl := CompileTemplate(tmpl, SupportedPlaceholders)
		level := plan.Levels[depth]

		var reqs []evaluateReq
		for _, qItem := range queue {
			for _, word := range level.Words {
				newVars := make(map[string]string)
				for k, v := range qItem.vars {
					newVars[k] = v
				}
				newVars[level.Placeholder] = word
				reqs = append(reqs, evaluateReq{word: word, vars: newVars})
			}
		}

		var nextQueue []queueItem
		pending := r.evaluateLevel(ctx, e, cTmpl, depth < len(plan.Levels)-1, reqs)

		for p := range pending {
			if p.err != nil {
				r.recordPruned(plan, depth)
				continue
			}
			res := <-p.ch

			if ctx.Err() == nil {
				if res.Accepted || res.Err != nil {
					yield(res)
				}
				if !res.Accepted && depth < len(plan.Levels)-1 {
					r.recordPruned(plan, depth)
				}
			}
			if ctx.Err() != nil {
				continue
			}

			if res.Accepted {
				if depth < len(plan.Levels)-1 {
					nextQueue = append(nextQueue, queueItem{vars: p.vars})
				}
			}
		}

		queue = nextQueue
	}

	return nil
}

type priorityTask struct {
	depth int
	vars  map[string]string
}

type priorityResult struct {
	task priorityTask
	res  Result
	err  error
}

func (r *Runner) runPriority(ctx context.Context, e *Executor, plan TraversalPlan, yield ResultCallback) error {
	if len(plan.Levels) == 0 {
		return nil
	}

	// Precompile templates for each depth level.
	cTmpls := make([]CompiledTemplate, len(plan.Levels))
	for d := 0; d < len(plan.Levels); d++ {
		tmpl := plan.TruncateTemplate(r.TargetURL, d)
		cTmpls[d] = CompileTemplate(tmpl, SupportedPlaceholders)
	}

	// Seed priority deque with level 0 tasks in original wordlist order.
	level0 := plan.Levels[0]
	var deque []priorityTask
	for _, word := range level0.Words {
		vars := map[string]string{level0.Placeholder: word}
		deque = append(deque, priorityTask{
			depth: 0,
			vars:  vars,
		})
	}

	maxInFlight := r.Threads
	if maxInFlight <= 0 {
		maxInFlight = e.workers
	}
	if maxInFlight <= 0 {
		maxInFlight = 10
	}

	inFlight := 0
	completionChan := make(chan priorityResult, maxInFlight*4)

	for len(deque) > 0 || inFlight > 0 {
		// Dispatch tasks while executor has capacity AND priority deque is not empty.
		for len(deque) > 0 && inFlight < maxInFlight {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			task := deque[0]
			deque = deque[1:]

			job, err := r.buildJob(cTmpls[task.depth], task.vars)
			if err != nil {
				r.recordPruned(plan, task.depth)
				continue
			}
			job.IsProbing = (task.depth < len(plan.Levels)-1)

			asyncCh, err := e.ExecuteAsync(job)
			if err != nil {
				r.recordPruned(plan, task.depth)
				continue
			}

			inFlight++
			go func(t priorityTask, ch <-chan Result) {
				select {
				case <-ctx.Done():
					return
				case res, ok := <-ch:
					if !ok {
						return
					}
					select {
					case completionChan <- priorityResult{task: t, res: res}:
					case <-ctx.Done():
					}
				}
			}(task, asyncCh)
		}

		if inFlight == 0 && len(deque) == 0 {
			break
		}

		// Wait for next completed result asynchronously.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case comp := <-completionChan:
			inFlight--

			if comp.err != nil {
				r.recordPruned(plan, comp.task.depth)
				continue
			}

			res := comp.res
			if ctx.Err() == nil {
				if res.Accepted || res.Err != nil {
					yield(res)
				}
				if !res.Accepted && comp.task.depth < len(plan.Levels)-1 {
					r.recordPruned(plan, comp.task.depth)
				}
			}
			if ctx.Err() != nil {
				continue
			}

			// If accepted and deeper levels exist, enqueue child tasks to FRONT of priority deque
			if res.Accepted && comp.task.depth < len(plan.Levels)-1 {
				nextDepth := comp.task.depth + 1
				nextLevel := plan.Levels[nextDepth]

				var childTasks []priorityTask
				for _, word := range nextLevel.Words {
					childVars := make(map[string]string, len(comp.task.vars)+1)
					for k, v := range comp.task.vars {
						childVars[k] = v
					}
					childVars[nextLevel.Placeholder] = word
					childTasks = append(childTasks, priorityTask{
						depth: nextDepth,
						vars:  childVars,
					})
				}

				// Push child tasks to the FRONT of the priority deque
				deque = append(childTasks, deque...)
			}
		}
	}

	return nil
}

func (r *Runner) runAdaptive(ctx context.Context, e *Executor, plan TraversalPlan, yield ResultCallback) error {
	if r.AdaptiveEngine == nil {
		r.AdaptiveEngine = adaptive.NewEngine(r.TargetURL, r.Client, r.Cache, r.Quiet)
	}
	engine := r.AdaptiveEngine
	r.Summary = engine.Summary
	if err := engine.Discover(ctx); err != nil {
		return err
	}

	type payload struct {
		word string
		idx  int
	}

	var adaptVisit func(currentDepth int, vars map[string]string, parentPaths []string, parentIndices []int)

	adaptVisit = func(currentDepth int, vars map[string]string, parentPaths []string, parentIndices []int) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		tmpl := plan.TruncateTemplate(r.TargetURL, currentDepth)
		cTmpl := CompileTemplate(tmpl, SupportedPlaceholders)
		level := plan.Levels[currentDepth]

		// Score and sort candidates
		payloads := make([]payload, len(level.Words))
		for i, w := range level.Words {
			payloads[i] = payload{word: w, idx: i}
		}

		if currentDepth == 0 && !r.Quiet {
			type scoredItem struct {
				word  string
				score int
			}
			var scoredItems []scoredItem
			for _, w := range level.Words {
				score := engine.GetScore(w, nil, 1, "")
				scoredItems = append(scoredItems, scoredItem{word: w, score: score})
			}
			sort.Slice(scoredItems, func(i, j int) bool {
				return scoredItems[i].score > scoredItems[j].score
			})
			var buf strings.Builder
			buf.WriteString("\r\nPriority scores:\r\n\r\n")
			printed := 0
			for _, item := range scoredItems {
				if item.score <= 0 {
					break
				}
				buf.WriteString(fmt.Sprintf("    %-15s %s\r\n", item.word, presentation.Number(int64(item.score))))
				printed++
				if printed >= 15 {
					break
				}
			}
			if printed == 0 {
				buf.WriteString("    (no prioritized items)\r\n")
			}
			buf.WriteString("\r\nTraversal decisions:\r\n\r\n")
			r.printInfo(buf.String())
		}

		sort.SliceStable(payloads, func(i, j int) bool {
			scoreI := engine.GetScore(payloads[i].word, parentPaths, currentDepth+1, "")
			scoreJ := engine.GetScore(payloads[j].word, parentPaths, currentDepth+1, "")
			return scoreI > scoreJ
		})

		if currentDepth < len(plan.Levels)-1 {
			for _, p := range payloads {
				select {
				case <-ctx.Done():
					return
				default:
				}

				newVars := make(map[string]string)
				for k, v := range vars {
					newVars[k] = v
				}
				newVars[level.Placeholder] = p.word

				reqs := []evaluateReq{{word: p.word, idx: p.idx, vars: newVars}}
				pending := r.evaluateLevel(ctx, e, cTmpl, true, reqs)

				for item := range pending {
					if item.err != nil {
						continue
					}
					res := <-item.ch
					if ctx.Err() == nil {
						if res.Accepted || res.Err != nil {
							yield(res)
						}
						if res.Accepted {
							parts := strings.Split(strings.TrimRight(res.URL, "/"), "/")
							var val string
							if len(parts) > 0 {
								val = parts[len(parts)-1]
							} else {
								val = p.word
							}

							ct := res.Headers.Get("Content-Type")
							sigs := engine.GetSignals(p.word, parentPaths, currentDepth+1, ct)
							dec := engine.SelectTraversal(sigs)

							engine.Summary.RecordTraversal(dec.Policy)

							if !r.Quiet {
								ruleDesc := fmt.Sprintf("%s (rule: %s)", dec.Policy, dec.Rule)
								r.printInfo(fmt.Sprintf("    %-12s %s", "/"+val, ruleDesc))
							}

							newParentPaths := make([]string, len(parentPaths))
							copy(newParentPaths, parentPaths)
							newParentPaths = append(newParentPaths, p.word)

							newParentIndices := make([]int, len(parentIndices))
							copy(newParentIndices, parentIndices)
							newParentIndices = append(newParentIndices, p.idx)

							if dec.Policy == types.PolicyDFS || dec.Policy == types.PolicyBFS {
								adaptVisit(currentDepth+1, newVars, newParentPaths, newParentIndices)
							} else if dec.Policy == types.PolicyEager {
								var eagerVisit func(d int, v map[string]string, indices []int)
								var eagerWg sync.WaitGroup

								eagerVisit = func(d int, v map[string]string, indices []int) {
									if d == len(plan.Levels) {
										eagerWg.Add(1)
										go func(vCopy map[string]string, idxCopy []int) {
											defer eagerWg.Done()
											job, err := r.buildJob(r.compiledReq.targetURL, vCopy)
											if err != nil {
												return
											}
											res, err := e.Execute(job)
											if err == nil {
												if res.Accepted || res.Err != nil {
													yield(res)
												}
											}
										}(v, indices)
										return
									}

									curLevel := plan.Levels[d]
									for i, w := range curLevel.Words {
										vCopy := make(map[string]string)
										for k, val := range v {
											vCopy[k] = val
										}
										vCopy[curLevel.Placeholder] = w

										idxCopy := make([]int, len(indices))
										copy(idxCopy, indices)
										idxCopy = append(idxCopy, i)

										eagerVisit(d+1, vCopy, idxCopy)
									}
								}
								eagerVisit(currentDepth+1, newVars, newParentIndices)
								eagerWg.Wait()
							}
						}
					}
				}
			}
			return
		}

		// Leaf level (currentDepth == len(plan.Levels)-1): evaluate all candidates in parallel and yield live.
		var reqs []evaluateReq
		for _, p := range payloads {
			newVars := make(map[string]string)
			for k, v := range vars {
				newVars[k] = v
			}
			newVars[level.Placeholder] = p.word
			reqs = append(reqs, evaluateReq{word: p.word, idx: p.idx, vars: newVars})
		}

		pending := r.evaluateLevel(ctx, e, cTmpl, false, reqs)
		for p := range pending {
			if p.err != nil {
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

	adaptVisit(0, make(map[string]string), nil, nil)
	return nil
}
