package wordlist

import (
	"context"
	"sync"
)

// SliceProvider is an optional interface for readers that can supply words directly as a slice.
type SliceProvider interface {
	Words(ctx context.Context) ([]string, error)
}

// SliceReader wraps any Reader and caches its loaded words as a string slice,
// allowing fast, zero-allocation iteration across multiple consumers.
type SliceReader struct {
	reader Reader
	once   sync.Once
	words  []string
	err    error
}

// NewSliceReader returns a SliceReader wrapping r. If r is already a SliceProvider or *SliceReader, it returns r.
func NewSliceReader(r Reader) Reader {
	if r == nil {
		return nil
	}
	if sr, ok := r.(*SliceReader); ok {
		return sr
	}
	return &SliceReader{reader: r}
}

func (sr *SliceReader) Words(ctx context.Context) ([]string, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	sr.once.Do(func() {
		capacity := 1024
		if c, ok := sr.reader.(Countable); ok {
			if n, err := c.Count(); err == nil && n > 0 {
				capacity = n
			}
		}

		ch := make(chan string, 4096)
		errCh := make(chan error, 1)
		go func() {
			defer close(ch)
			errCh <- sr.reader.Read(ctx, ch)
		}()

		words := make([]string, 0, capacity)
		for w := range ch {
			words = append(words, w)
		}
		sr.words = words
		sr.err = <-errCh
	})
	return sr.words, sr.err
}

func (sr *SliceReader) Read(ctx context.Context, out chan<- string) error {
	words, err := sr.Words(ctx)
	if err != nil && len(words) == 0 {
		return err
	}
	for _, w := range words {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- w:
		}
	}
	return err
}

func (sr *SliceReader) Count() (int, error) {
	words, err := sr.Words(context.Background())
	return len(words), err
}

// LoadWords extracts all entries from a Reader as a string slice,
// leveraging SliceProvider if implemented, or wrapping with SliceReader.
func LoadWords(ctx context.Context, r Reader) ([]string, error) {
	if r == nil {
		return nil, nil
	}
	if sp, ok := r.(SliceProvider); ok {
		return sp.Words(ctx)
	}
	sr := NewSliceReader(r)
	if sp, ok := sr.(SliceProvider); ok {
		return sp.Words(ctx)
	}
	return nil, nil
}
