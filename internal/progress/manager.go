package progress

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/unsubble/searchit/internal/console"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/presentation"
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

	isStatsActive  bool
	deferredOutput []engine.Result
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

	// Initialize the terminal regions at startup.
	// SetupRegions is no longer needed.
	_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		m.Renderer.RenderInto(w, m.Collector.Snapshot())
	})

	for {
		select {
		case <-ctx.Done():
			// Final render before the goroutine exits.
			// The global TM lock ensures this cannot interleave with Close().
			_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
				m.Renderer.RenderInto(w, m.Collector.Snapshot())
			})
			return

		case <-ticker.C:
			if !m.isStatsActive {
				_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
					m.Renderer.RenderInto(w, m.Collector.Snapshot())
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
						m.Renderer.RenderInto(w, m.Collector.Snapshot())
					})
				}

			case console.CommandStats:
				m.isStatsActive = true
				// Switch owner to Statistics and render the full-screen report.
				_ = m.Renderer.Clear()
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
			case console.CommandStopTarget, console.CommandAbortAll:
				// Graceful stop/abort is handled by the caller cancelling the context.
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

// restoreDashboard prints the resuming message and switches back to the progress owner.
func (m *Manager) restoreDashboard() {
	_ = m.TM.SwitchOwner(terminal.OwnerStatistics, terminal.OwnerProgress)
	m.isStatsActive = false

	_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Resuming scan...")
		fmt.Fprintln(w)
		m.Renderer.Reset()

		// Flush any results that were discovered while the stats view was blocking terminal output.
		for _, r := range m.deferredOutput {
			var path string
			if r.StatusCode >= 200 && r.StatusCode <= 299 {
				path = r.URL
			} else {
				path = presentation.RelativeURL(m.Renderer.Target, r.URL)
			}

			sizeStr := "     -"
			if r.Length >= 0 {
				sizeStr = fmt.Sprintf("%8s", presentation.Size(r.Length))
			}

			loc := r.RedirectURL
			if loc == "" && r.Headers != nil {
				if rawLoc := r.Headers.Get("Location"); rawLoc != "" {
					loc = presentation.ResolveRedirect(r.URL, rawLoc)
				}
			}

			var formatted string
			if r.StatusCode >= 300 && r.StatusCode <= 399 && loc != "" {
				formatted = fmt.Sprintf("%-3d │ %s │ %s", r.StatusCode, sizeStr, presentation.Redirect(m.Renderer.Target, r.URL, loc))
			} else {
				formatted = fmt.Sprintf("%-3d │ %s │ %s", r.StatusCode, sizeStr, path)
			}

			fmt.Fprintln(w, formatted)
			m.Renderer.AddResultLocked(r.StatusCode, r.URL, formatted)

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
		m.deferredOutput = nil

		// Re-render the fresh dashboard below the flushed results
		m.Renderer.RenderInto(w, m.Collector.Snapshot())
	})
}

// HandleResult routes a discovered result through the terminal manager.
// If the statistics view is active, the result is buffered.
// Otherwise: clear the dashboard, print the result, re-render the dashboard.
// All of this happens atomically under the TM global lock.
func (m *Manager) HandleResult(r engine.Result) {
	if m.isStatsActive {
		m.deferredOutput = append(m.deferredOutput, r)
		return
	}

	var path string
	if r.StatusCode >= 200 && r.StatusCode <= 299 {
		path = r.URL
	} else {
		path = presentation.RelativeURL(m.Renderer.Target, r.URL)
	}

	sizeStr := "     -"
	if r.Length >= 0 {
		sizeStr = fmt.Sprintf("%8s", presentation.Size(r.Length))
	}

	loc := r.RedirectURL
	if loc == "" && r.Headers != nil {
		if rawLoc := r.Headers.Get("Location"); rawLoc != "" {
			loc = presentation.ResolveRedirect(r.URL, rawLoc)
		}
	}

	var formatted string
	if r.StatusCode >= 300 && r.StatusCode <= 399 && loc != "" {
		formatted = fmt.Sprintf("%-3d │ %s │ %s", r.StatusCode, sizeStr, presentation.Redirect(m.Renderer.Target, r.URL, loc))
	} else {
		formatted = fmt.Sprintf("%-3d │ %s │ %s", r.StatusCode, sizeStr, path)
	}

	// Coordinate permanent terminal output safely under the TM lock
	_ = m.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		// 1. Erase current progress block
		m.Renderer.ClearInto(w)

		// 2. Print finding permanently
		fmt.Fprintln(w, formatted)

		// 3. Keep a copy in the recent buffer for the full-screen Stats view
		m.Renderer.AddResultLocked(r.StatusCode, r.URL, formatted)

		// 4. Redraw progress block underneath
		m.Renderer.RenderInto(w, m.Collector.Snapshot())
	})

	// Emit non-terminal formatter outputs (e.g. MultiFormatter to file)
	if m.Formatter != nil {
		_ = m.Formatter.Print(r)
	}
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
