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

type discoveryEntry struct {
	StatusCode int
	Path       string
	Formatted  string
}

const (
	ProgressPanelHeight = 5
)

// ANSIRenderer renders statistics snapshots using live ANSI updates in the terminal.
//
// All writes go through the TerminalManager (TM), which holds the single global
// output lock. The renderer's own mu protects only the recent-discoveries slice
// and lastLineCount, which may be accessed from multiple goroutines concurrently.
type ANSIRenderer struct {
	TM       *terminal.Manager
	Target   string
	Profiles []string
	Mode     string
	limit    int
	frozen   bool // true if the underlying writer is a real TTY
	logCount int

	mu            sync.Mutex // protects recent + lastLineCount + lastProgCount only
	recent        []discoveryEntry
	lastLineCount int
	lastProgCount int

	IsPaused func() bool
}

// NewANSIRenderer creates a new ANSIRenderer.
// The cursor-hide escape is emitted via TM.Emit so it goes through the global lock.
func NewANSIRenderer(tm *terminal.Manager, target string, profiles []string, mode string, logCount int) *ANSIRenderer {
	tr := &ANSIRenderer{
		TM:       tm,
		Target:   target,
		Profiles: profiles,
		Mode:     mode,
		limit:    5,
		frozen:   true, // assume TTY; non-TTY writes are harmless
		logCount: logCount,
	}
	// Hide cursor — goes through global lock.
	_ = tm.Emit(terminal.OwnerProgress, func(w io.Writer) {
		fmt.Fprint(w, "\033[?25l")
	})
	return tr
}

// AddResultLocked records a successful discovery without locking (caller must hold TM lock).
func (tr *ANSIRenderer) AddResultLocked(statusCode int, urlStr string, formatted string) {
	path := extractPath(tr.Target, urlStr)
	entry := discoveryEntry{StatusCode: statusCode, Path: path, Formatted: formatted}

	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.logCount <= 0 {
		return
	}
	if len(tr.recent) >= tr.logCount {
		tr.recent = append(tr.recent[1:], entry)
	} else {
		tr.recent = append(tr.recent, entry)
	}
}

// AddResult records a successful discovery (called from worker goroutines).
func (tr *ANSIRenderer) AddResult(statusCode int, urlStr string, formatted string) {
	tr.AddResultLocked(statusCode, urlStr, formatted)
}

// RecentEntries returns a copy of the recent discoveries.
func (tr *ANSIRenderer) RecentEntries() []discoveryEntry {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	copied := make([]discoveryEntry, len(tr.recent))
	copy(copied, tr.recent)
	return copied
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

// renderInto draws the progress panel. Called INSIDE an Emit closure.
func (tr *ANSIRenderer) renderInto(w io.Writer, snap stats.Snapshot) {
	tr.mu.Lock()
	lastLines := tr.lastLineCount
	tr.mu.Unlock()

	var lines []string

	progLines := tr.renderCompactProgress(snap)
	lines = append(lines, progLines...)

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
	tr.lastProgCount = len(progLines)
	tr.mu.Unlock()
}

func (tr *ANSIRenderer) renderCompactProgress(snap stats.Snapshot) []string {
	// 5-line compact progress
	// Line 1: Target: <url>
	// Line 2: Progress: [██████████░░░░░░░░░░] 50.0%
	// Line 3: Jobs: X Remaining / Y Candidates (Z Workers)
	// Line 4: Metrics: X Req/s • Y Elapsed • Z ETA
	// Line 5: Results: X Findings • Y Errors • Z Retries

	var p float64
	totalJobs := snap.TotalCandidates
	if totalJobs == 0 {
		totalJobs = snap.RequestsSent // fallback for undefined search space
	}
	remainingJobs := totalJobs - snap.SearchSpaceProgress
	if remainingJobs < 0 {
		remainingJobs = 0
	}

	if totalJobs > 0 {
		if snap.SearchSpaceProgress >= totalJobs {
			p = 100.0
		} else {
			p = float64(snap.SearchSpaceProgress) / float64(totalJobs) * 100.0
		}
	}
	if p < 0.0 {
		p = 0.0
	}
	if p > 100.0 {
		p = 100.0
	}
	bar := progressBar(p, 20)
	elapsed := presentation.Duration(time.Since(snap.StartTime))

	eta := "-"
	elapsedSec := time.Since(snap.StartTime).Seconds()
	if elapsedSec > 2.0 && snap.SearchSpaceProgress > 0 && remainingJobs > 0 {
		candidatesPerSec := float64(snap.SearchSpaceProgress) / elapsedSec
		etaSecs := float64(remainingJobs) / candidatesPerSec
		if etaSecs < 1.0 {
			etaSecs = 1.0
		}
		eta = presentation.Duration(time.Duration(math.Ceil(etaSecs)) * time.Second)
	}

	completedRequests := snap.ResponsesReceived + snap.RequestsFailed
	isWarmingUp := false
	if snap.ActiveWorkers > 0 || remainingJobs > 0 {
		if completedRequests < (snap.ActiveWorkers*3) || completedRequests == 0 {
			isWarmingUp = true
		}
	}

	var metrics string
	if isWarmingUp {
		metrics = fmt.Sprintf("Metrics: Warming up... • %s Elapsed", elapsed)
	} else {
		metrics = fmt.Sprintf("Metrics: %.0f Req/s • %s Elapsed • %s ETA", snap.CurrentRequestsPerSecond, elapsed, eta)
	}

	controls := "[p] Pause │ [q] Stop │ [a] Abort │ [s] Stats"
	if tr.IsPaused != nil && tr.IsPaused() {
		controls = "[p] Resume │ [q] Stop │ [a] Abort │ [s] Stats"
	}

	return []string{
		fmt.Sprintf("Progress: [%s] %.1f%% (Search Space)", bar, p),
		fmt.Sprintf("Jobs: %s Generated (%d Workers)", presentation.Number(snap.JobsProduced), snap.ActiveWorkers),
		metrics,
		fmt.Sprintf("Results: %s Findings • %s Errors • %s Retries", presentation.Number(snap.Discovered), presentation.Number(snap.RequestsFailed), presentation.Number(snap.Retries)),
		controls,
	}
}

func extractPath(target, urlStr string) string {
	idx := strings.Index(urlStr, target)
	if idx != -1 {
		return urlStr[idx+len(target):]
	}
	return urlStr
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
