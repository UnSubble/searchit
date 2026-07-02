package wordlist

import (
	"context"

	"github.com/unsubble/searchit/internal/engine"
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
}

func (p Producer) Produce(ctx context.Context, jobs chan<- engine.Job) error {
	defer close(jobs)

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

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case word, ok := <-words:
			if !ok {
				return <-readErr
			}
			cleaned, ok := CleanWord(word, p.NormalizePaths, p.CollapseSlashes)
			if !ok {
				continue
			}
			url, err := Join(p.BaseURL, cleaned)
			if err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case jobs <- engine.Job{URL: url}:
			}
		}
	}
}
