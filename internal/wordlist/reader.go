package wordlist

import "context"

// Reader streams word entries to an output channel.
// It does not own the channel - the caller manages its lifetime.
// Implementations must respect ctx cancellation.
type Reader interface {
	Read(ctx context.Context, out chan<- string) error
}

// Countable is an optional interface that Readers can implement to report
// the total number of words they contain (useful for progress bar estimation).
type Countable interface {
	Count() (int, error)
}
