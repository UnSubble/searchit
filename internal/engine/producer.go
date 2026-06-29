package engine

import "context"

// Producer generates Jobs and sends them to the provided channel.
// Implementations must close jobs when done and respect ctx cancellation.
type Producer interface {
	Produce(ctx context.Context, jobs chan<- Job) error
}

// SliceProducer emits one Job per URL from a fixed slice.
type SliceProducer struct {
	URLs []string
}

func (p SliceProducer) Produce(ctx context.Context, jobs chan<- Job) error {
	defer close(jobs)
	for _, u := range p.URLs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case jobs <- Job{URL: u}:
		}
	}
	return nil
}
