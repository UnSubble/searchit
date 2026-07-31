package progress

import (
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/presentation"
	"github.com/unsubble/searchit/internal/stats"
)

const (
	ProgressPanelHeight = 5
)

// ANSIRenderer renders statistics snapshots using live ANSI updates in the terminal.
//
// All writes go through the TerminalManager (TM), which holds the single global
// output lock.
type ANSIRenderer struct {
	TM                *terminal.Manager
	Target            string
	Profiles          []string
	Mode              string
	Method            string
	HTTPVersion       string
	limit             int
	frozen            bool // true if the underlying writer is a real TTY
	ConfiguredThreads int

	mu            sync.Mutex // protects lastLineCount + lastProgCount only
	lastLineCount int
	lastProgCount int

	IsPaused      func() bool
	IsStatsActive func() bool
}

// NewANSIRenderer creates a new ANSIRenderer.
// The cursor-hide escape is emitted via TM.Emit so it goes through the global lock.
func NewANSIRenderer(tm *terminal.Manager, target string, profiles []string, mode string) *ANSIRenderer {
	tr := &ANSIRenderer{
		TM:       tm,
		Target:   target,
		Profiles: profiles,
		Mode:     mode,
		limit:    5,
		frozen:   true, // assume TTY; non-TTY writes are harmless
	}
	// Hide cursor — goes through global lock.
	_ = tm.Emit(terminal.OwnerProgress, func(w io.Writer) {
		fmt.Fprint(w, "\033[?25l")
	})
	return tr
}

// ResetLineCount resets the internal line counter. Call this after an external
// operation cleared the terminal so the next Render doesn't try to cursor-up
// over non-existent lines.
func (tr *ANSIRenderer) ResetLineCount() {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.lastLineCount = 0
}

// Close restores the terminal state (cursor visible).
// Must be called while the caller holds OwnerProgress on TM.
func (tr *ANSIRenderer) Close(owner terminal.Owner) error {
	return tr.TM.Emit(owner, func(w io.Writer) {
		tr.mu.Lock()
		progLines := tr.lastProgCount
		tr.lastLineCount = 0
		tr.lastProgCount = 0
		tr.mu.Unlock()

		if progLines > 0 && tr.frozen {
			fmt.Fprintf(w, "\r\033[%dA\033[J", progLines)
		}

		// Emitting \n ensures the cursor is firmly at column 0 for subsequent blocks.
		fmt.Fprint(w, "\033[?25h\n")
	})
}

// Render issues a full progress-block render via TM.Emit.
// Safe to call from any goroutine; serialized by the TM global lock.
func (tr *ANSIRenderer) Render(snap stats.Snapshot) error {
	return tr.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		tr.renderInto(w, snap)
	})
}

// Clear erases the current progress block via TM.Emit.
func (tr *ANSIRenderer) Clear() error {
	return tr.TM.Emit(terminal.OwnerProgress, func(w io.Writer) {
		tr.ClearInto(w)
	})
}

// ClearInto erases the current progress block.
// Must be called inside TM.Emit.
func (tr *ANSIRenderer) ClearInto(w io.Writer) {
	tr.mu.Lock()
	lastLines := tr.lastLineCount
	tr.lastLineCount = 0
	tr.lastProgCount = 0
	tr.mu.Unlock()

	if lastLines > 0 && tr.frozen {
		// Move up lastLines, then erase everything below the cursor to the end of screen.
		fmt.Fprintf(w, "\r\033[%dA\033[J", lastLines)
	}
}

// RenderInto renders the progress block directly into w.
// Must be called inside TM.Emit.
func (tr *ANSIRenderer) RenderInto(w io.Writer, snap stats.Snapshot) {
	tr.renderInto(w, snap)
}

// ─── Internal write helpers (called inside TM.Emit — TM lock already held) ───

// clearInto is removed. The block renderer handles its own clears via \033[K.

// SetupRegions is removed.
func (tr *ANSIRenderer) SetupRegions(w io.Writer) {}

// TeardownRegions is removed.
func (tr *ANSIRenderer) TeardownRegions(w io.Writer) {}

// Reset clears the renderer's state so it doesn't attempt to erase previous output.
func (tr *ANSIRenderer) Reset() {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.lastLineCount = 0
	tr.lastProgCount = 0
}

// renderInto draws the progress panel or statistics overlay. Called INSIDE an Emit closure.
func (tr *ANSIRenderer) renderInto(w io.Writer, snap stats.Snapshot) {
	tr.mu.Lock()
	lastLines := tr.lastLineCount
	tr.mu.Unlock()

	var lines []string

	if tr.IsStatsActive != nil && tr.IsStatsActive() {
		lines = statsReport(tr.TM.ContentWidth(), snap, tr.ConfiguredThreads, tr.Target, tr.Profiles, tr.Mode, tr.Method, tr.HTTPVersion)
	} else {
		lines = tr.renderCompactProgress(snap)
	}

	// Print lines
	if !tr.frozen {
		return
	}

	// Disable line wrap to guarantee 1 logical line == 1 physical line
	fmt.Fprint(w, "\033[?7l")

	// Move cursor UP by lastLines to rewrite the block
	if lastLines > 0 {
		fmt.Fprintf(w, "\r\033[%dA", lastLines)
	}

	for _, line := range lines {
		// \r ensures column 0, \033[K erases line, print line, and newline
		fmt.Fprintf(w, "\r\033[K%s\n", line)
	}

	// If the new frame is shorter than the previous, erase the leftover lines
	if lastLines > len(lines) {
		fmt.Fprintf(w, "\033[J")
	}

	// Re-enable line wrap
	fmt.Fprint(w, "\033[?7h")

	tr.mu.Lock()
	tr.lastLineCount = len(lines)
	tr.lastProgCount = len(lines)
	tr.mu.Unlock()
}

func (tr *ANSIRenderer) renderCompactProgress(snap stats.Snapshot) []string {
	elapsed := presentation.Duration(time.Since(snap.StartTime))
	elapsedSec := time.Since(snap.StartTime).Seconds()
	completedRequests := snap.ResponsesReceived + snap.RequestsFailed
	isWarmingUp := elapsedSec < 2.0 && completedRequests == 0

	controls := "[p] Pause │ [q] Stop │ [a] Abort │ [s] Stats"
	if tr.IsPaused != nil && tr.IsPaused() {
		controls = "[p] Resume │ [q] Stop │ [a] Abort │ [s] Stats"
	}

	if snap.IsFinite {
		totalWork := snap.TotalWork
		completed := snap.Tried + snap.Skipped
		var p float64
		if totalWork > 0 {
			p = float64(completed) / float64(totalWork) * 100.0
			if p > 100.0 {
				p = 100.0
			}
		}
		if p < 0.0 {
			p = 0.0
		}

		remainingWork := totalWork - completed
		if remainingWork < 0 {
			remainingWork = 0
		}

		bar := progressBar(p, 20)

		eta := "-"
		if elapsedSec > 2.0 && completed > 0 && remainingWork > 0 {
			completedPerSec := float64(completed) / elapsedSec
			etaSecs := float64(remainingWork) / completedPerSec
			if etaSecs < 1.0 {
				etaSecs = 1.0
			}
			eta = presentation.Duration(time.Duration(math.Ceil(etaSecs)) * time.Second)
		}

		var metrics string
		if isWarmingUp {
			metrics = fmt.Sprintf("Metrics: Warming up... • %s Elapsed", elapsed)
		} else {
			metrics = fmt.Sprintf("Metrics: %.0f Req/s • %s Elapsed • %s ETA", snap.CurrentRequestsPerSecond, elapsed, eta)
		}

		return []string{
			fmt.Sprintf("Progress: %s %d / %d (%.1f%%)", bar, completed, totalWork, p),
			fmt.Sprintf("Jobs: %s Requests Sent (%d Workers)", presentation.Number(snap.RequestsSent), snap.ActiveWorkers),
			metrics,
			fmt.Sprintf("Results: %s Findings • %s Errors • %s Retries", presentation.Number(snap.Discovered), presentation.Number(snap.RequestsFailed), presentation.Number(snap.Retries)),
			controls,
		}
	}

	// Open-ended / Recursive scan rendering
	var metrics string
	if isWarmingUp {
		metrics = fmt.Sprintf("Metrics: Warming up... • %s Elapsed", elapsed)
	} else {
		metrics = fmt.Sprintf("Metrics: %.0f Req/s • %s Elapsed", snap.CurrentRequestsPerSecond, elapsed)
	}

	return []string{
		fmt.Sprintf("Requests Sent: %s (%d Workers)", presentation.Number(snap.RequestsSent), snap.ActiveWorkers),
		fmt.Sprintf("Directories: Discovered: %s │ Queued: %s", presentation.Number(snap.DirectoriesDiscovered), presentation.Number(snap.DirectoriesQueued)),
		metrics,
		fmt.Sprintf("Results: %s Findings • %s Errors • %s Retries", presentation.Number(snap.Discovered), presentation.Number(snap.RequestsFailed), presentation.Number(snap.Retries)),
		controls,
	}
}

func progressBar(p float64, width int) string {
	filled := int((p / 100.0) * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	empty := width - filled
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", empty) + "]"
}
