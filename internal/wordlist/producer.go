package wordlist

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/extensions"
	"github.com/unsubble/searchit/internal/stats"
)

// DefaultWordBuffer is the capacity of the internal word channel.
// Bounded buffering avoids turning large wordlists into unbounded memory usage
// while keeping workers fed during disk I/O latency spikes.
const DefaultWordBuffer = 4096

// Producer satisfies engine.Producer. It reads from a Reader, converts each
// word into a fully-qualified URL, and emits engine.Jobs.
type Producer struct {
	BaseURL         string
	Reader          Reader
	NormalizePaths  bool
	CollapseSlashes bool
	Extensions      []string
	Collector       *stats.Collector
	PauseBlocker    func(context.Context) error
}

func (p Producer) Produce(ctx context.Context, jobs chan<- engine.Job) error {
	defer func() {
		close(jobs)
		stats.GlobalInstrumentation.LogEvent("jobs channel close")
		atomic.AddInt64(&stats.GlobalInstrumentation.ProducerExit, 1)
		stats.GlobalInstrumentation.LogEvent("producer exited")
	}()

	// Validate the base before touching the reader so a bad URL is caught
	// immediately rather than after consuming part of the wordlist.
	if _, err := Join(p.BaseURL, ""); err != nil {
		return err
	}

	words := make(chan string, DefaultWordBuffer)
	readErr := make(chan error, 1)

	go func() {
		defer close(words)
		readErr <- p.Reader.Read(ctx, words)
	}()

	var wg sync.WaitGroup
	const numProducers = 8
	errCh := make(chan error, numProducers)

	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					stats.GlobalInstrumentation.LogEvent("context cancellation")
					errCh <- ctx.Err()
					return
				case word, ok := <-words:
					if !ok || ctx.Err() != nil {
						return
					}
					cleaned, ok := CleanWord(word, p.NormalizePaths, p.CollapseSlashes)
					if !ok {
						continue
					}

					if p.PauseBlocker != nil {
						if err := p.PauseBlocker(ctx); err != nil {
							errCh <- err
							return
						}
					}

					variants := extensions.GenerateVariants(cleaned, p.Extensions)
					for _, variant := range variants {
						url, err := Join(p.BaseURL, variant)
						if err != nil {
							atomic.AddInt64(&stats.GlobalInstrumentation.InvalidWords, 1)
							if p.Collector != nil {
								p.Collector.RecordInvalidWord()
							}
							continue
						}

						select {
						case <-ctx.Done():
							stats.GlobalInstrumentation.LogEvent("context cancellation")
							errCh <- ctx.Err()
							return
						case jobs <- engine.Job{URL: url}:
							atomic.AddInt64(&stats.GlobalInstrumentation.JobsSubmitted, 1)
							if p.Collector != nil {
								p.Collector.RecordJobProduced()
								p.Collector.AddTotalCandidates(1)
							}
						}
					}
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return <-readErr
}
