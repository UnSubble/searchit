package progress

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

// ExecuteAbove clears the progress block, calls fn(w) where w is the TM-locked
// terminal writer, then redraws the progress block. All three operations
// (clear, finding output, redraw) are written through the same writer inside
// the TM mutex, so ANSI cursor sequences and finding text are never interleaved
// across separate file descriptors.
//
// A bare \r is emitted before fn(w) so that the cursor is guaranteed to be at
// column 0 regardless of the terminal's current autowrap state. This neutralises
// the effect of \033[?7l (disable autowrap) that renderInto may have left active
// if the previous render cycle did not complete its \033[?7h restore before the
// ClearInto sequence ran.
// crlfWriter translates '\n' to '\r\n' to ensure correct rendering when
// the terminal is in raw mode and OPOST is disabled.
type crlfWriter struct {
	w io.Writer
}

func (c *crlfWriter) Write(p []byte) (int, error) {
	replaced := bytes.ReplaceAll(p, []byte("\n"), []byte("\r\n"))
	replaced = bytes.ReplaceAll(replaced, []byte("\r\r\n"), []byte("\r\n"))
	_, err := c.w.Write(replaced)
	return len(p), err
}

func (m *Manager) ExecuteAbove(fn func(w io.Writer)) {
	_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		m.Renderer.ClearInto(w)
		// Hard-reset to column 0 before writing the finding. This is a defensive
		// measure: \r is idempotent when the cursor is already at column 0, and it
		// corrects column drift when \033[?7l (nowrap) is still active from a prior
		// render cycle, because in nowrap mode \n does not reset the column but \r does.
		fmt.Fprint(w, "\r")

		// Wrap w so that any \n emitted by fn translates to \r\n.
		cw := &crlfWriter{w: w}
		fn(cw)

		m.Renderer.RenderInto(w, m.Collector.Snapshot())
	})
}

// PrintAbove clears the progress block, writes msg through the terminal
// manager's locked writer (the same io.Writer used by all progress output),
// then redraws the progress block. Use this for informational messages that
// must not race with the live progress UI. Unlike ExecuteAbove, which passes
// the TM writer into the callback, PrintAbove writes the string directly
// inside the TM lock so no write bypasses the terminal manager.
func (m *Manager) PrintAbove(msg string) {
	_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		m.Renderer.ClearInto(w)
		cw := &crlfWriter{w: w}
		fmt.Fprint(cw, "\r")
		fmt.Fprint(cw, msg)
		// Ensure the message ends with a clean CRLF before the redrawn block.
		if len(msg) == 0 || msg[len(msg)-1] != '\n' {
			fmt.Fprint(cw, "\n")
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
