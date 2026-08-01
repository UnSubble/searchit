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
			// Final render before the goroutine exits.
			_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
				m.Renderer.RenderInto(w, m.Collector.Snapshot())
			})
			return

		case <-ticker.C:
			m.Collector.Sample()
			_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
				m.Renderer.RenderInto(w, m.Collector.Snapshot())
			})

		case cmd, ok := <-cmdChan:
			if !ok {
				cmdChan = nil
				break
			}
			switch cmd {
			case console.CommandProgress:
				m.mu.Lock()
				m.isStatsActive = false
				m.mu.Unlock()
				_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
					m.Renderer.RenderInto(w, m.Collector.Snapshot())
				})

			case console.CommandStats:
				m.mu.Lock()
				m.isStatsActive = !m.isStatsActive
				m.mu.Unlock()
				_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
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

// PrintStats renders the full-screen statistics report.
// Exported for use by cmd layer in non-interactive mode.
func (m *Manager) PrintStats() {
	snap := m.Collector.Snapshot()
	_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		RenderStatsViewFull(w, m.TM.ContentWidth(), snap, m.ConfiguredThreads,
			m.Renderer.Target, m.Renderer.Profiles, m.Renderer.Mode, m.Renderer.Method, m.Renderer.HTTPVersion)
	})
}
