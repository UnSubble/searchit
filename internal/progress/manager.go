package progress

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/unsubble/searchit/internal/console"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/stats"
)

// Manager handles periodic rendering of runtime statistics.
// It also acts as a central terminal/output manager to prevent concurrent prints to stdout.
type Manager struct {
	Collector         *stats.Collector
	Renderer          Renderer
	Interval          time.Duration
	ConfiguredThreads int

	// Central terminal/output manager state
	mu              sync.Mutex
	Formatter       output.Formatter
	isStatsActive   bool
	bufferedResults []engine.Result
}

// NewManager creates a new progress Manager.
func NewManager(collector *stats.Collector, renderer Renderer, interval time.Duration) *Manager {
	if interval <= 0 {
		interval = 1 * time.Second
	}
	return &Manager{
		Collector: collector,
		Renderer:  renderer,
		Interval:  interval,
	}
}

// Start launches the periodic refresh loop. Blocks until the context is cancelled.
// It also listens for user commands from cmdChan if provided.
//
// The statistics view is implemented as a nested select loop inside Start().
// While the stats screen is active, all ticker ticks are absorbed and no
// progress renders occur. The outer loop resumes only after the user presses
// any key (CommandProgress) or the context is cancelled.
func (m *Manager) Start(ctx context.Context, cmdChan <-chan console.Command) {
	ticker := time.NewTicker(m.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final render before exit.
			m.mu.Lock()
			_ = m.Renderer.Render(m.Collector.Snapshot())
			m.mu.Unlock()
			return

		case <-ticker.C:
			m.mu.Lock()
			if !m.isStatsActive {
				_ = m.Renderer.Render(m.Collector.Snapshot())
			}
			m.mu.Unlock()

		case cmd, ok := <-cmdChan:
			if !ok {
				cmdChan = nil
				break
			}
			switch cmd {
			case console.CommandProgress:
				m.mu.Lock()
				if !m.isStatsActive {
					_ = m.Renderer.Render(m.Collector.Snapshot())
				}
				m.mu.Unlock()

			case console.CommandStats:
				m.mu.Lock()
				m.isStatsActive = true
				m.mu.Unlock()

				// 1. Render the statistics report (clears terminal internally).
				m.renderStatsReport()

				// 2. Block here until the user presses any key (controller
				//    sends CommandProgress) or the scan context is cancelled.
				//    All ticker ticks are silently discarded while we wait.
				m.awaitStatsExit(ctx, ticker, &cmdChan)
			}
		}
	}
}

// awaitStatsExit blocks the event loop while the statistics view is visible.
// Ticker ticks are absorbed. The loop exits on CommandProgress (any key),
// context cancellation, or cmdChan closure. After exiting, the live dashboard
// is restored by clearing the terminal and re-rendering.
func (m *Manager) awaitStatsExit(
	ctx context.Context,
	ticker *time.Ticker,
	cmdChan *<-chan console.Command,
) {
	for {
		select {
		case <-ctx.Done():
			// Scan is over; no need to restore.
			return

		case <-ticker.C:
			// Absorb: do not render while the stats view is active.

		case cmd2, ok2 := <-*cmdChan:
			if !ok2 {
				*cmdChan = nil
				// Channel closed; restore dashboard and stop waiting.
				m.restoreDashboard()
				return
			}
			switch cmd2 {
			case console.CommandProgress:
				// Any key was pressed: restore the live dashboard.
				m.restoreDashboard()
				return
			case console.CommandStop:
				// Graceful stop is handled by the caller cancelling the
				// context; we do not need to act here.
			}
			// Any other command while in stats view is ignored.
		}
	}
}

// renderStatsReport renders the full-screen statistics report.
// For ANSI renderers, the terminal is cleared inside RenderStatsViewFull.
func (m *Manager) renderStatsReport() {
	m.mu.Lock()
	defer m.mu.Unlock()

	tr, isANSI := m.Renderer.(*ANSIRenderer)
	if !isANSI {
		m.printStatsLocked()
		return
	}

	var w io.Writer = os.Stdout
	if tr.Writer != nil {
		w = tr.Writer
	}

	snap := m.Collector.Snapshot()
	recent := tr.RecentEntries()
	RenderStatsViewFull(w, snap, m.ConfiguredThreads, recent, tr.Target, tr.Profiles, tr.Mode)
}

// restoreDashboard clears the terminal, prints all buffered results, and re-renders the live dashboard.
func (m *Manager) restoreDashboard() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.isStatsActive = false

	if tr, ok := m.Renderer.(*ANSIRenderer); ok {
		// Clear terminal screen and home cursor
		fmt.Fprint(tr.Writer, "\033[2J\033[H")
		tr.ResetLineCount()

		// Print all buffered results
		for _, r := range m.bufferedResults {
			if m.Formatter != nil {
				_ = m.Formatter.Print(r)
			}
		}
		m.bufferedResults = nil

		// Render the live dashboard
		_ = tr.Render(m.Collector.Snapshot())
	} else {
		// Non-ANSI renderer
		for _, r := range m.bufferedResults {
			if m.Formatter != nil {
				_ = m.Formatter.Print(r)
			}
		}
		m.bufferedResults = nil
		_ = m.Renderer.Render(m.Collector.Snapshot())
	}
}

// PrintStats is the backward-compatible path used by non-ANSI renderers and tests.
func (m *Manager) PrintStats() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.printStatsLocked()
}

func (m *Manager) printStatsLocked() {
	var w io.Writer = os.Stdout
	if tr, ok := m.Renderer.(*ANSIRenderer); ok {
		w = tr.Writer
	} else if tr, ok := m.Renderer.(*TextRenderer); ok {
		w = tr.Writer
	}

	snap := m.Collector.Snapshot()

	var recent []discoveryEntry
	if tr, ok := m.Renderer.(*ANSIRenderer); ok {
		recent = tr.RecentEntries()
	}

	target, profiles, mode := "", []string(nil), ""
	if tr, ok := m.Renderer.(*ANSIRenderer); ok {
		target, profiles, mode = tr.Target, tr.Profiles, tr.Mode
	}

	RenderStatsViewFull(w, snap, m.ConfiguredThreads, recent, target, profiles, mode)

	if _, ok := m.Renderer.(*ANSIRenderer); ok {
		_ = m.Renderer.Render(snap)
	}
}

// RecordResult feeds a discovered url into the renderer if it supports results registration.
func (m *Manager) RecordResult(statusCode int, urlStr string) {
	if tr, ok := m.Renderer.(*ANSIRenderer); ok {
		tr.AddResult(statusCode, urlStr)
	}
}

// HandleResult routes a discovered result through the terminal/output manager.
// If stats view is active, the result is buffered in memory.
// If live dashboard is active, the dashboard is cleared, the result is formatted and printed,
// and the dashboard is re-rendered.
func (m *Manager) HandleResult(r engine.Result) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Record in ANSIRenderer's recent list
	if tr, ok := m.Renderer.(*ANSIRenderer); ok {
		tr.AddResult(r.StatusCode, r.URL)
	}

	// 2. Buffer if stats screen is active
	if m.isStatsActive {
		m.bufferedResults = append(m.bufferedResults, r)
		return
	}

	// 3. Otherwise print result cleanly
	if tr, ok := m.Renderer.(*ANSIRenderer); ok {
		tr.Clear()
		if m.Formatter != nil {
			_ = m.Formatter.Print(r)
		}
		_ = tr.Render(m.Collector.Snapshot())
	} else {
		if m.Formatter != nil {
			_ = m.Formatter.Print(r)
		}
	}
}
