package recursion

import (
	"fmt"
	"strings"
)

// Strategy controls insertion order on the frontier.
type Strategy int

const (
	BFS Strategy = iota
	DFS
)

// ParseStrategy parses a string representation into a Strategy.
// It accepts "bfs" and "dfs" case-insensitively, returning an error for other inputs.
func ParseStrategy(s string) (Strategy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "bfs":
		return BFS, nil
	case "dfs":
		return DFS, nil
	default:
		return BFS, fmt.Errorf("unknown strategy: %q", s)
	}
}

func (s Strategy) String() string {
	switch s {
	case BFS:
		return "bfs"
	case DFS:
		return "dfs"
	default:
		return "unknown"
	}
}

const DefaultJobBuffer = 2048

// Frontier is a scheduling ring buffer for pending jobs.
// BFS appends to the back; DFS prepends to the front.
// Single-threaded ownership eliminates synchronization overhead.
type Frontier struct {
	strategy Strategy
	buf      []Generator
	head     int
	size     int
}

// NewFrontier creates a Frontier with the default initial capacity.
func NewFrontier(s Strategy) *Frontier {
	return &Frontier{
		strategy: s,
		buf:      make([]Generator, DefaultJobBuffer),
	}
}

// NewFrontierWithCapacity creates a Frontier with the specified initial capacity.
func NewFrontierWithCapacity(s Strategy, capacity int) *Frontier {
	if capacity <= 0 {
		capacity = DefaultJobBuffer
	}
	return &Frontier{
		strategy: s,
		buf:      make([]Generator, capacity),
	}
}

// Push enqueues a generator. If the buffer is full, it is doubled.
func (f *Frontier) Push(gen Generator) {
	if f.size == len(f.buf) {
		f.grow()
	}

	if f.strategy == DFS {
		f.head = (f.head - 1 + len(f.buf)) % len(f.buf)
		f.buf[f.head] = gen
	} else {
		tail := (f.head + f.size) % len(f.buf)
		f.buf[tail] = gen
	}
	f.size++
}

// PushFront enqueues a generator at the head of the buffer, giving it the highest priority.
func (f *Frontier) PushFront(gen Generator) {
	if f.size == len(f.buf) {
		f.grow()
	}

	f.head = (f.head - 1 + len(f.buf)) % len(f.buf)
	f.buf[f.head] = gen
	f.size++
}

// Pop dequeues the next generator from the head of the buffer.
func (f *Frontier) Pop() {
	if f.size == 0 {
		return
	}
	f.buf[f.head] = nil // Release reference for GC
	f.head = (f.head + 1) % len(f.buf)
	f.size--

	if f.size > 0 && f.size == len(f.buf)/4 && len(f.buf) > DefaultJobBuffer {
		f.shrink()
	}
}

// Len returns the number of active elements in the buffer.
func (f *Frontier) Len() int {
	return f.size
}

// Peek returns the next generator without removing it.
func (f *Frontier) Peek() (Generator, bool) {
	if f.size == 0 {
		return nil, false
	}
	return f.buf[f.head], true
}

// grow doubles the buffer capacity. Elements are copied in logical order
// from head to tail starting at index 0 of the new slice.
func (f *Frontier) grow() {
	newCap := len(f.buf) * 2
	newBuf := make([]Generator, newCap)
	for i := 0; i < f.size; i++ {
		newBuf[i] = f.buf[(f.head+i)%len(f.buf)]
	}
	f.buf = newBuf
	f.head = 0
}

func (f *Frontier) shrink() {
	newCap := len(f.buf) / 2
	newBuf := make([]Generator, newCap)
	for i := 0; i < f.size; i++ {
		newBuf[i] = f.buf[(f.head+i)%len(f.buf)]
	}
	f.buf = newBuf
	f.head = 0
}
