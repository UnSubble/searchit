package progress

import (
	"context"
	"io"
	"time"

	"github.com/unsubble/searchit/internal/console"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/stats"
)

// Manager handles periodic rendering of runtime statistics.
//
// All terminal output goes through TM.Emit(OwnerProgress, ...) which holds
// the single global output lock. There is no local mutex: the TerminalManager's
// sync.Mutex is the ONE global output lock for the entire process.
type Manager struct {
	TM                *terminal.Manager
	Collector         *stats.Collector
	Renderer          *ANSIRenderer
	Interval          time.Duration
	ConfiguredThreads int
	Formatter         output.Formatter

	isStatsActive   bool
	bufferedResults []engine.Result
}

// NewManager creates a new progress Manager.
// tm must have OwnerProgress already acquired by the caller before Start is called.
func NewManager(tm *terminal.Manager, collector *stats.Collector, renderer *ANSIRenderer, interval time.Duration) *Manager {
	if interval <= 0 {
		interval = 1 * time.Second
	}
	return &Manager{
		TM:        tm,
		Collector: collector,
		Renderer:  renderer,
		Interval:  interval,
	}
}

// Start launches the periodic refresh loop. Blocks until the context is cancelled.
// The caller must hold OwnerProgress on TM before calling Start and release it
// after Start returns.
func (m *Manager) Start(ctx context.Context, cmdChan <-chan console.Command) {
	ticker := time.NewTicker(m.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final render before the goroutine exits.
			// The global TM lock ensures this cannot interleave with Close().
			_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
				m.Renderer.renderInto(w, m.Collector.Snapshot())
			})
			return

		case <-ticker.C:
			if !m.isStatsActive {
				_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
					m.Renderer.renderInto(w, m.Collector.Snapshot())
				})
			}

		case cmd, ok := <-cmdChan:
			if !ok {
				cmdChan = nil
				break
			}
			switch cmd {
			case console.CommandProgress:
				if !m.isStatsActive {
					_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
						m.Renderer.renderInto(w, m.Collector.Snapshot())
					})
				}

			case console.CommandStats:
				m.isStatsActive = true
				// Switch owner to Statistics and render the full-screen report.
				_ = m.TM.SwitchOwner(terminal.OwnerProgress, terminal.OwnerStatistics)
				m.renderStatsReport()
				m.awaitStatsExit(ctx, ticker, &cmdChan)
			}
		}
	}
}

// awaitStatsExit blocks while the statistics view is visible.
func (m *Manager) awaitStatsExit(
	ctx context.Context,
	ticker *time.Ticker,
	cmdChan *<-chan console.Command,
) {
	for {
		select {
		case <-ctx.Done():
			// Switch back before exiting.
			_ = m.TM.SwitchOwner(terminal.OwnerStatistics, terminal.OwnerProgress)
			return

		case <-ticker.C:
			// Absorb: do not render while the stats view is active.

		case cmd2, ok2 := <-*cmdChan:
			if !ok2 {
				*cmdChan = nil
				m.restoreDashboard()
				return
			}
			switch cmd2 {
			case console.CommandProgress:
				m.restoreDashboard()
				return
			case console.CommandStop:
				// Graceful stop is handled by the caller cancelling the context.
			}
		}
	}
}

// renderStatsReport renders the full-screen statistics view under OwnerStatistics.
func (m *Manager) renderStatsReport() {
	snap := m.Collector.Snapshot()
	recent := m.Renderer.RecentEntries()
	_ = m.TM.Emit(terminal.OwnerStatistics, func(w io.Writer) {
		RenderStatsViewFull(w, m.TM.ContentWidth(), snap, m.ConfiguredThreads, recent,
			m.Renderer.Target, m.Renderer.Profiles, m.Renderer.Mode)
	})
}

// restoreDashboard clears the stats view, prints buffered results, re-renders dashboard.
func (m *Manager) restoreDashboard() {
	_ = m.TM.SwitchOwner(terminal.OwnerStatistics, terminal.OwnerProgress)
	m.isStatsActive = false

	_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		m.Renderer.clearInto(w)
		for _, r := range m.bufferedResults {
			if m.Formatter != nil {
				if pt, ok := m.Formatter.(interface {
					PrintTo(io.Writer, engine.Result) error
				}); ok {
					_ = pt.PrintTo(w, r)
				} else {
					_ = m.Formatter.Print(r)
				}
			}
		}
		m.bufferedResults = nil
		m.Renderer.renderInto(w, m.Collector.Snapshot())
	})
}

// RecordResult feeds a discovered URL into the renderer's recent list.
// Safe to call from any goroutine (ANSIRenderer.AddResult is protected by its own mu).
func (m *Manager) RecordResult(statusCode int, urlStr string) {
	m.Renderer.AddResult(statusCode, urlStr)
}

// HandleResult routes a discovered result through the terminal manager.
// If the statistics view is active, the result is buffered.
// Otherwise: clear the dashboard, print the result, re-render the dashboard.
// All of this happens atomically under the TM global lock.
func (m *Manager) HandleResult(r engine.Result) {
	// Record in the renderer's recent list (protected by its own mu).
	m.Renderer.AddResult(r.StatusCode, r.URL)

	if m.isStatsActive {
		m.bufferedResults = append(m.bufferedResults, r)
		return
	}

	_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		m.Renderer.clearInto(w)
		if m.Formatter != nil {
			if pt, ok := m.Formatter.(interface {
				PrintTo(io.Writer, engine.Result) error
			}); ok {
				_ = pt.PrintTo(w, r)
			} else {
				_ = m.Formatter.Print(r)
			}
		}
		m.Renderer.renderInto(w, m.Collector.Snapshot())
	})
}

// PrintStats renders the full-screen statistics report.
// Exported for use by cmd layer in non-interactive mode.
func (m *Manager) PrintStats() {
	snap := m.Collector.Snapshot()
	recent := m.Renderer.RecentEntries()
	_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		RenderStatsViewFull(w, m.TM.ContentWidth(), snap, m.ConfiguredThreads, recent,
			m.Renderer.Target, m.Renderer.Profiles, m.Renderer.Mode)
	})
}
