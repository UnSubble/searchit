package stats

import "sync/atomic"

// Counter represents a simple concurrency-safe atomic integer counter.
type Counter struct {
	value int64
}

// Add atomically adds the given delta to the counter.
func (c *Counter) Add(delta int64) {
	atomic.AddInt64(&c.value, delta)
}

// Increment atomically increments the counter by 1.
func (c *Counter) Increment() {
	atomic.AddInt64(&c.value, 1)
}

// Value atomically loads and returns the current counter value.
func (c *Counter) Value() int64 {
	return atomic.LoadInt64(&c.value)
}
