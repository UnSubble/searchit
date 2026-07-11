package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/unsubble/searchit/internal/console"
	"github.com/unsubble/searchit/internal/stats"
	"golang.org/x/term"
)

type discoveryEntry struct {
	statusCode int
	path       string
}

// ANSIRenderer renders statistics snapshots using live ANSI updates in the terminal.
type ANSIRenderer struct {
	Writer        io.Writer
	Target        string
	Profiles      []string
	Mode          string
	limit         int
	lastLineCount int

	mu     sync.Mutex
	recent []discoveryEntry
	frozen bool
}

// NewANSIRenderer creates a new ANSIRenderer writing to the provided Writer.
// Automatically hides terminal cursor.
func NewANSIRenderer(w io.Writer, target string, profiles []string, mode string) *ANSIRenderer {
	if w == nil {
		w = os.Stdout
	}
	// Hide cursor: \033[?25l
	fmt.Fprint(w, "\033[?25l")

	frozen := false
	if f, ok := w.(*os.File); ok && console.IsTerminal(f.Fd()) {
		frozen = true
	}

	return &ANSIRenderer{
		Writer:   w,
		Target:   target,
		Profiles: profiles,
		Mode:     mode,
		limit:    5,
		frozen:   frozen,
	}
}

// AddResult records a successful discovery path.
func (tr *ANSIRenderer) AddResult(statusCode int, urlStr string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	path := extractPath(tr.Target, urlStr)
	entry := discoveryEntry{statusCode: statusCode, path: path}

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

// Close restores the terminal state by clearing the progress view and showing the cursor.
func (tr *ANSIRenderer) Close() error {
	tr.Clear()
	// Show cursor (\033[?25h)
	fmt.Fprint(tr.Writer, "\033[?25h")
	return nil
}

// ResetLineCount resets the internal line counter so the next Render call
// prints fresh without attempting to cursor-up over non-existent lines.
// Call this after an external operation that cleared the terminal.
func (tr *ANSIRenderer) ResetLineCount() {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.lastLineCount = 0
}

// Clear erases the rendered progress block by moving the cursor up and clearing lines.
func (tr *ANSIRenderer) Clear() {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if tr.frozen && tr.lastLineCount > 0 {
		fmt.Fprintf(tr.Writer, "\033[%dA", tr.lastLineCount)
		for i := 0; i < tr.lastLineCount; i++ {
			fmt.Fprint(tr.Writer, "\r\033[K\n")
		}
		fmt.Fprintf(tr.Writer, "\033[%dA", tr.lastLineCount)
		tr.lastLineCount = 0
	}
}

// Render overwrites the progress block by composing sections and passing them to draw().
func (tr *ANSIRenderer) Render(snap stats.Snapshot) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	// 1. Get Terminal Width dynamically
	termWidth := 80
	if f, ok := tr.Writer.(*os.File); ok && console.IsTerminal(f.Fd()) {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil {
			termWidth = w
		}
	}

	// 2. Content Width (clamp between 72 and 96 columns)
	contentWidth := clamp(termWidth-8, 72, 96)

	// 3. Profiles string helper
	profilesStr := "none"
	if len(tr.Profiles) > 0 {
		profilesStr = strings.Join(tr.Profiles, " -> ")
	}

	// 4. Fixed divider around 48 characters
	divider := strings.Repeat("─", 48)

	// 5. Compose the layout parts
	var lines []string
	lines = append(lines, tr.renderHeader(profilesStr, contentWidth)...)
	lines = append(lines, "")
	lines = append(lines, "Progress")
	lines = append(lines, divider)
	lines = append(lines, tr.renderStats(snap)...)
	lines = append(lines, "")
	lines = append(lines, "Recent discoveries")
	lines = append(lines, divider)
	lines = append(lines, tr.renderDiscoveries(contentWidth)...)
	lines = append(lines, "")
	lines = append(lines, tr.renderControls())

	// 6. Draw with a small constant left margin (2 spaces)
	tr.draw(lines, "  ")

	return nil
}

func (tr *ANSIRenderer) renderHeader(profilesStr string, contentWidth int) []string {
	formatHeaderLine := func(label, value string) string {
		maxValLen := contentWidth - 12
		val := value
		if len(val) > maxValLen && maxValLen > 3 {
			val = val[:maxValLen-3] + "..."
		}
		return fmt.Sprintf("%-12s%s", label, val)
	}
	return []string{
		formatHeaderLine("Target", tr.Target),
		formatHeaderLine("Profiles", profilesStr),
		formatHeaderLine("Mode", tr.Mode),
	}
}

func (tr *ANSIRenderer) renderStats(snap stats.Snapshot) []string {
	elapsed := time.Since(snap.StartTime)
	h := int(elapsed.Hours())
	m := int(elapsed.Minutes()) % 60
	s := int(elapsed.Seconds()) % 60
	elapsedStr := fmt.Sprintf("%02d:%02d:%02d", h, m, s)

	etaStr := "-"
	if snap.RequestsPerSecond > 0 && snap.QueuedJobs > 0 {
		etaSecs := float64(snap.QueuedJobs) / snap.RequestsPerSecond
		eh := int(etaSecs / 3600)
		em := int(etaSecs/60) % 60
		es := int(etaSecs) % 60
		etaStr = fmt.Sprintf("%02d:%02d:%02d", eh, em, es)
	}

	formatStatsRow := func(leftKey, leftVal, rightKey, rightVal string) string {
		leftText := fmt.Sprintf("%-12s%-14s", leftKey, leftVal)
		rightText := fmt.Sprintf("%-12s%s", rightKey, rightVal)
		return leftText + rightText
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
		formatStatsRow("Latency", formatLatency(snap.AverageLatency), "Retries", fmt.Sprintf("%d / %d", snap.Retries, snap.Redirects)),
	}
}

func (tr *ANSIRenderer) renderDiscoveries(contentWidth int) []string {
	if len(tr.recent) == 0 {
		return []string{"No discoveries yet."}
	}
	var lines []string
	for _, entry := range tr.recent {
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

func (tr *ANSIRenderer) draw(lines []string, marginStr string) {
	// 1. Move cursor back up if we rendered before
	if tr.frozen && tr.lastLineCount > 0 {
		fmt.Fprintf(tr.Writer, "\033[%dA", tr.lastLineCount)
	}

	// 2. Print the new lines
	for _, line := range lines {
		fmt.Fprintf(tr.Writer, "\r\033[K%s%s\n", marginStr, line)
	}

	// 3. Clear any remaining lines from the previous draw
	if tr.frozen && tr.lastLineCount > len(lines) {
		diff := tr.lastLineCount - len(lines)
		for i := 0; i < diff; i++ {
			fmt.Fprint(tr.Writer, "\r\033[K\n")
		}
		// Move cursor back up by the diff so it sits exactly at the end of the printed lines
		fmt.Fprintf(tr.Writer, "\033[%dA", diff)
	}

	if tr.frozen {
		tr.lastLineCount = len(lines)
	}
}

func formatLatency(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
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

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
