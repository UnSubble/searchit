package recursion

import (
	"context"
	"net/http"
	"strings"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/wordlist"
)

// ShouldRecurse reports whether a response with the given status code
// should trigger a recursive scan of that URL.
// 403 is included because protected directories often have enumerable children.
func ShouldRecurse(status int) bool {
	switch status {
	case 200, 301, 302, 403:
		return true
	}
	return false
}

// Manager orchestrates recursive directory scanning.
// It owns the frontier, the visited set, and all traversal decisions.
// Workers remain stateless execution units — they never know recursion exists.
type Manager struct {
	client   *http.Client
	exclude  status.Filters
	reader   wordlist.Reader
	strategy Strategy
	maxDepth uint16
}

func NewManager(client *http.Client, exclude status.Filters, reader wordlist.Reader, strategy Strategy, maxDepth uint16) *Manager {
	return &Manager{
		client:   client,
		exclude:  exclude,
		reader:   reader,
		strategy: strategy,
		maxDepth: maxDepth,
	}
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
				frontier.Push(engine.Job{URL: u, Depth: 0})
			}
		}

		jobs := make(chan engine.Job, workers)
		results := engine.Start(ctx, m.client, m.exclude, workers, jobs)

		// pending counts jobs dispatched to workers but not yet returned.
		// The loop ends when the frontier is empty and no in-flight work remains.
		pending := 0

		for frontier.Len() > 0 || pending > 0 {
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

	if result.Depth >= m.maxDepth || !ShouldRecurse(result.StatusCode) {
		return
	}

	words := make(chan string, wordlist.DefaultWordBuffer)
	readErr := make(chan error, 1)
	go func() {
		defer close(words)
		readErr <- m.reader.Read(ctx, words)
	}()

	for word := range words {
		childURL, err := wordlist.Join(result.URL, word)
		if err != nil {
			continue
		}
		key := normalizeURL(childURL)
		if _, seen := visited[key]; seen {
			continue
		}
		visited[key] = struct{}{}
		frontier.Push(engine.Job{URL: childURL, Depth: result.Depth + 1})
	}
	if err := <-readErr; err != nil && err != context.Canceled {
		// Reader failures during recursion do not invalidate already-scheduled children.
	}
}

// normalizeURL strips trailing slashes so /admin and /admin/ map to the same key.
func normalizeURL(u string) string {
	return strings.TrimRight(u, "/")
}
