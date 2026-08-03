package progress

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/unsubble/searchit/internal/console"
	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/stats"
)

// Manager handles periodic rendering of runtime statistics and progress overlays.
//
// All terminal output goes through TM.Emit(OwnerProgress, ...) which holds the process-wide output lock.
type Manager struct {
	TM                *terminal.Manager
	Collector         *stats.Collector
	Renderer          *ANSIRenderer
	Interval          time.Duration
	ConfiguredThreads int

	isStatsActive bool
	mu            sync.Mutex
}

// NewManager creates a new progress Manager.
// tm must have OwnerProgress already acquired by the caller before Start is called.
func NewManager(tm *terminal.Manager, collector *stats.Collector, renderer *ANSIRenderer, interval time.Duration) *Manager {
	if interval <= 0 {
		interval = 1 * time.Second
	}
	m := &Manager{
		TM:        tm,
		Collector: collector,
		Renderer:  renderer,
		Interval:  interval,
	}
	renderer.IsStatsActive = func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.isStatsActive
	}
	return m
}

// Start launches the periodic refresh loop. Blocks until the context is cancelled.
// The caller must hold OwnerProgress on TM before calling Start and release it
// after Start returns.
func (m *Manager) Start(ctx context.Context, cmdChan <-chan console.Command) {
	m.Renderer.ConfiguredThreads = m.ConfiguredThreads

	ticker := time.NewTicker(m.Interval)
	defer ticker.Stop()

	// Initialize the terminal regions at startup.
	_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		m.Renderer.RenderInto(w, m.Collector.Snapshot())
	})

	for {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			m.isStatsActive = false
			m.mu.Unlock()
			_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
				m.Renderer.ClearInto(w)
			})
			return

		case <-ticker.C:
			m.Collector.Sample()
			_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
				m.Renderer.RenderInto(w, m.Collector.Snapshot())
			})

		case cmd, ok := <-cmdChan:
			if !ok {
				m.mu.Lock()
				m.isStatsActive = false
				m.mu.Unlock()
				_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
					m.Renderer.ClearInto(w)
				})
				return
			}
			switch cmd {
			case console.CommandProgress:
				m.mu.Lock()
				m.isStatsActive = false
				m.mu.Unlock()
				_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
					m.Renderer.ClearInto(w)
					m.Renderer.RenderInto(w, m.Collector.Snapshot())
				})

			case console.CommandStats:
				m.mu.Lock()
				m.isStatsActive = !m.isStatsActive
				m.mu.Unlock()
				_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
					m.Renderer.ClearInto(w)
					m.Renderer.RenderInto(w, m.Collector.Snapshot())
				})
			}
		}
	}
}

// ExecuteAbove clears the progress block on stderr, executes fn, and redraws the progress block on stderr.
func (m *Manager) ExecuteAbove(fn func()) {
	_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		m.Renderer.ClearInto(w)
		_ = os.Stderr.Sync()
		fmt.Fprint(os.Stdout, "\r")
		fn()
		_ = os.Stdout.Sync()
		m.Renderer.RenderInto(w, m.Collector.Snapshot())
		_ = os.Stderr.Sync()
	})
}

// PrintAbove clears the progress block, writes msg through the terminal
// manager's locked writer (the same io.Writer used by all progress output),
// then redraws the progress block. Use this for informational messages that
// must not race with the live progress UI. Unlike ExecuteAbove, which hands a
// raw func() to the caller, PrintAbove keeps the write inside the TM lock so
// no direct os.Stderr write bypasses the terminal manager.
func (m *Manager) PrintAbove(msg string) {
	_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		m.Renderer.ClearInto(w)
		fmt.Fprint(w, msg)
		// Ensure the message ends with a clean CRLF before the redrawn block.
		if len(msg) == 0 || msg[len(msg)-1] != '\n' {
			fmt.Fprint(w, "\r\n")
		}
		m.Renderer.RenderInto(w, m.Collector.Snapshot())
	})
}

// PrintStats renders the full-screen statistics report.
// Exported for use by cmd layer in non-interactive mode.
func (m *Manager) PrintStats() {
	snap := m.Collector.Snapshot()
	_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		RenderStatsViewFull(w, m.TM.ContentWidth(), snap, m.ConfiguredThreads,
			m.Renderer.Target, m.Renderer.Profiles, m.Renderer.Mode, m.Renderer.Method, m.Renderer.HTTPVersion)
	})
}
