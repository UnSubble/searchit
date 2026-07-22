package progress

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/stats"
)

type discoveryEntry struct {
	statusCode int
	path       string
}

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

	mu            sync.Mutex // protects recent + lastLineCount only
	recent        []discoveryEntry
	lastLineCount int
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

// AddResult records a successful discovery (called from worker goroutines).
func (tr *ANSIRenderer) AddResult(statusCode int, urlStr string) {
	path := extractPath(tr.Target, urlStr)
	entry := discoveryEntry{statusCode: statusCode, path: path}

	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.recent) >= tr.limit {
		tr.recent = append(tr.recent[1:], entry)
	} else {
		tr.recent = append(tr.recent, entry)
	}
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
		tr.clearInto(w)
		// Emitting \n ensures the cursor is firmly at column 0 for subsequent blocks.
		fmt.Fprint(w, "\033[?25h\n")
		tr.mu.Lock()
		tr.lastLineCount = 0
		tr.mu.Unlock()
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
		tr.clearInto(w)
	})
}

// ─── Internal write helpers (called inside TM.Emit — TM lock already held) ───

// clearInto erases the rendered progress block. Called INSIDE an Emit closure.
func (tr *ANSIRenderer) clearInto(w io.Writer) {
	tr.mu.Lock()
	lc := tr.lastLineCount
	tr.mu.Unlock()

	if tr.frozen && lc > 0 {
		fmt.Fprintf(w, "\033[%dA", lc)
		for i := 0; i < lc; i++ {
			fmt.Fprint(w, "\r\033[K\n")
		}
		fmt.Fprintf(w, "\033[%dA", lc)
		tr.mu.Lock()
		tr.lastLineCount = 0
		tr.mu.Unlock()
	}
}

// renderInto draws the full progress block. Called INSIDE an Emit closure.
func (tr *ANSIRenderer) renderInto(w io.Writer, snap stats.Snapshot) {
	contentWidth := tr.TM.ContentWidth()

	profilesStr := "none"
	if len(tr.Profiles) > 0 {
		profilesStr = strings.Join(tr.Profiles, " -> ")
	}

	divider := terminal.Separator(contentWidth, terminal.ThinSeparatorChar)

	tr.mu.Lock()
	recentCopy := make([]discoveryEntry, len(tr.recent))
	copy(recentCopy, tr.recent)
	tr.mu.Unlock()

	var lines []string
	lines = append(lines, tr.renderHeader(profilesStr, contentWidth)...)
	lines = append(lines, "")
	lines = append(lines, "Progress")
	lines = append(lines, divider)
	lines = append(lines, tr.renderStats(snap)...)
	lines = append(lines, "")
	lines = append(lines, "Recent discoveries")
	lines = append(lines, divider)
	lines = append(lines, tr.renderDiscoveries(recentCopy, contentWidth)...)
	lines = append(lines, "")
	lines = append(lines, tr.renderControls())

	tr.drawInto(w, lines, "  ")
}

func (tr *ANSIRenderer) renderHeader(profilesStr string, contentWidth int) []string {
	formatHeaderLine := func(label, value string) string {
		return terminal.FormatDotRow(label, value, 12, contentWidth)
	}
	return []string{
		formatHeaderLine("Target", tr.Target),
		formatHeaderLine("Profiles", profilesStr),
		formatHeaderLine("Mode", tr.Mode),
	}
}

func (tr *ANSIRenderer) renderStats(snap stats.Snapshot) []string {
	elapsedStr := terminal.FormatElapsed(time.Since(snap.StartTime))
	etaStr := terminal.FormatETA(snap.QueuedJobs, snap.RequestsPerSecond)

	formatStatsRow := func(leftKey, leftVal, rightKey, rightVal string) string {
		return terminal.FormatTwoColumnRow(leftKey, leftVal, rightKey, rightVal)
	}

	return []string{
		formatStatsRow("Elapsed", elapsedStr, "ETA", etaStr),
		"",
		formatStatsRow("Requests", fmt.Sprintf("%d", snap.RequestsSent), "Responses", fmt.Sprintf("%d", snap.ResponsesReceived)),
		"",
		formatStatsRow("Req/s", fmt.Sprintf("%.0f", snap.RequestsPerSecond), "Workers", fmt.Sprintf("%d", snap.ActiveWorkers)),
		"",
		formatStatsRow("Queue", fmt.Sprintf("%d", snap.QueuedJobs), "Found", fmt.Sprintf("%d", snap.Discovered)),
		"",
		formatStatsRow("Filtered", fmt.Sprintf("%d", snap.RequestsFiltered), "Failed", fmt.Sprintf("%d", snap.RequestsFailed)),
		"",
		formatStatsRow("Latency", terminal.FormatLatency(snap.AverageLatency), "Retries", fmt.Sprintf("%d / %d", snap.Retries, snap.Redirects)),
	}
}

func (tr *ANSIRenderer) renderDiscoveries(recent []discoveryEntry, contentWidth int) []string {
	if len(recent) == 0 {
		return []string{"No discoveries yet."}
	}
	var lines []string
	for _, entry := range recent {
		statusPrefix := fmt.Sprintf("%d ", entry.statusCode)
		maxPathLen := contentWidth - len(statusPrefix)
		p := entry.path
		if len(p) > maxPathLen && maxPathLen > 3 {
			p = p[:maxPathLen-3] + "..."
		}
		lines = append(lines, statusPrefix+p)
	}
	return lines
}

func (tr *ANSIRenderer) renderControls() string {
	return "Controls: p Refresh · s Statistics · q Graceful stop"
}

// drawInto writes the progress block to w. Called INSIDE an Emit closure.
func (tr *ANSIRenderer) drawInto(w io.Writer, lines []string, marginStr string) {
	tr.mu.Lock()
	lc := tr.lastLineCount
	tr.mu.Unlock()

	if tr.frozen && lc > 0 {
		fmt.Fprintf(w, "\033[%dA", lc)
	}

	for _, line := range lines {
		fmt.Fprintf(w, "\r\033[K%s%s\n", marginStr, line)
	}

	if tr.frozen && lc > len(lines) {
		diff := lc - len(lines)
		for i := 0; i < diff; i++ {
			fmt.Fprint(w, "\r\033[K\n")
		}
		fmt.Fprintf(w, "\033[%dA", diff)
	}

	if tr.frozen {
		tr.mu.Lock()
		tr.lastLineCount = len(lines)
		tr.mu.Unlock()
	}
}

func extractPath(target, urlStr string) string {
	t := strings.TrimRight(target, "/")
	if strings.HasPrefix(urlStr, t) {
		p := urlStr[len(t):]
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		return p
	}
	return urlStr
}
