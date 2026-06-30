package wordlist

import "context"

// Reader streams word entries to an output channel.
// It does not own the channel - the caller manages its lifetime.
// Implementations must respect ctx cancellation.
type Reader interface {
	Read(ctx context.Context, out chan<- string) error
}
