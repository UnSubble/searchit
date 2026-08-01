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
	Priority
)

// ParseStrategy parses a string representation into a Strategy.
// It accepts "bfs", "dfs", and "priority" case-insensitively, returning an error for other inputs.
func ParseStrategy(s string) (Strategy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "bfs":
		return BFS, nil
	case "dfs":
		return DFS, nil
	case "priority":
		return Priority, nil
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
	case Priority:
		return "priority"
	default:
		return "unknown"
	}
}

const DefaultJobBuffer = 2048

// Frontier is a pure deque ring buffer for pending generators.
// Single-threaded ownership eliminates synchronization overhead.
// It does not contain traversal policy; the caller decides whether to PushFront or PushBack.
type Frontier struct {
	buf  []Generator
	head int
	size int
}

// NewFrontier creates a Frontier with the default initial capacity.
func NewFrontier(optionalStrategy ...Strategy) *Frontier {
	return &Frontier{
		buf: make([]Generator, DefaultJobBuffer),
	}
}

// NewFrontierWithCapacity creates a Frontier with the specified initial capacity.
func NewFrontierWithCapacity(args ...any) *Frontier {
	capacity := DefaultJobBuffer
	for _, arg := range args {
		if capVal, ok := arg.(int); ok && capVal > 0 {
			capacity = capVal
		}
	}
	return &Frontier{
		buf: make([]Generator, capacity),
	}
}

// PushBack enqueues a generator at the tail of the buffer (FIFO / BFS order).
func (f *Frontier) PushBack(gen Generator) {
	if f.size == len(f.buf) {
		f.grow()
	}

	tail := (f.head + f.size) % len(f.buf)
	f.buf[tail] = gen
	f.size++
}

// Push enqueues a generator at the tail of the buffer (alias for PushBack).
func (f *Frontier) Push(gen Generator) {
	f.PushBack(gen)
}

// PushFront enqueues a generator at the head of the buffer, giving it the highest priority (LIFO / Priority order).
func (f *Frontier) PushFront(gen Generator) {
	if f.size == len(f.buf) {
		f.grow()
	}

	f.head = (f.head - 1 + len(f.buf)) % len(f.buf)
	f.buf[f.head] = gen
	f.size++
}

// PopFront dequeues the next generator from the head of the buffer.
func (f *Frontier) PopFront() {
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

// Pop dequeues the next generator from the head of the buffer (alias for PopFront).
func (f *Frontier) Pop() {
	f.PopFront()
}

// Len returns the number of active elements in the buffer.
func (f *Frontier) Len() int {
	return f.size
}

// PeekFront returns the generator at the head without removing it.
func (f *Frontier) PeekFront() (Generator, bool) {
	if f.size == 0 {
		return nil, false
	}
	return f.buf[f.head], true
}

// Peek returns the next generator from the head without removing it (alias for PeekFront).
func (f *Frontier) Peek() (Generator, bool) {
	return f.PeekFront()
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
