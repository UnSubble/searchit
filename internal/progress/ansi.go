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
	Writer   io.Writer
	Target   string
	Profiles []string
	Mode     string
	limit    int

	mu     sync.Mutex
	recent []discoveryEntry
	frozen bool
}

const layoutHeight = 23

// NewANSIRenderer creates a new ANSIRenderer writing to the provided Writer.
// Automatically hides terminal cursor and freezes the top scrolling region.
func NewANSIRenderer(w io.Writer, target string, profiles []string, mode string) *ANSIRenderer {
	if w == nil {
		w = os.Stdout
	}
	// Hide cursor: \033[?25l
	fmt.Fprint(w, "\033[?25l")

	frozen := false
	if f, ok := w.(*os.File); ok && console.IsTerminal(f.Fd()) {
		// Reserve space for layoutHeight lines
		for i := 0; i < layoutHeight; i++ {
			fmt.Fprintln(w)
		}
		// Set scroll margin to start at layoutHeight + 1
		scrollStart := layoutHeight + 1
		fmt.Fprintf(w, "\033[%d;r", scrollStart)
		// Move cursor to start of scroll margin
		fmt.Fprintf(w, "\033[%d;1H", scrollStart)
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

// Close restores the terminal state by resetting margins and showing the cursor.
func (tr *ANSIRenderer) Close() error {
	// Reset margins (\033[r) and show cursor (\033[?25h)
	fmt.Fprint(tr.Writer, "\033[r\033[?25h")
	return nil
}

// Clear erases the frozen progress block.
func (tr *ANSIRenderer) Clear() {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if tr.frozen {
		fmt.Fprint(tr.Writer, "\033[s")
		fmt.Fprint(tr.Writer, "\033[1;1H")
		for i := 0; i < layoutHeight; i++ {
			fmt.Fprint(tr.Writer, "\033[K\n")
		}
		fmt.Fprint(tr.Writer, "\033[u")
	}
}

// Render overwrites the frozen top block using ANSI escape sequences.
func (tr *ANSIRenderer) Render(snap stats.Snapshot) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if tr.frozen {
		// Save cursor
		fmt.Fprint(tr.Writer, "\033[s")
		// Move to top-left
		fmt.Fprint(tr.Writer, "\033[1;1H")
	}

	// 1. Get Terminal Width dynamically
	termWidth := 80
	if f, ok := tr.Writer.(*os.File); ok && console.IsTerminal(f.Fd()) {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil {
			termWidth = w
		}
	}

	// 2. Bounded Content Width (approx 100 max, 80 min)
	contentWidth := termWidth
	if contentWidth > 100 {
		contentWidth = 100
	}
	if contentWidth < 80 {
		contentWidth = 80
	}

	// 3. Center the content area on wide terminals
	leftPadding := 0
	if termWidth > contentWidth {
		leftPadding = (termWidth - contentWidth) / 2
	}
	paddingStr := strings.Repeat(" ", leftPadding)

	// Formatting helpers within bounded content area
	formatHeaderLine := func(label, value string) string {
		maxValLen := contentWidth - 15
		val := value
		if len(val) > maxValLen && maxValLen > 3 {
			val = val[:maxValLen-3] + "..."
		}
		return fmt.Sprintf("%-15s%s", label, val)
	}

	formatStatsRow := func(leftKey, leftVal, rightKey, rightVal string) string {
		colWidth := contentWidth / 2

		leftText := fmt.Sprintf("%-15s%s", leftKey, leftVal)
		if len(leftText) > colWidth {
			leftText = leftText[:colWidth]
		} else {
			leftText = leftText + strings.Repeat(" ", colWidth-len(leftText))
		}

		rightText := fmt.Sprintf("%-16s%s", rightKey, rightVal)
		maxRightLen := contentWidth - colWidth
		if len(rightText) > maxRightLen {
			rightText = rightText[:maxRightLen]
		}

		return leftText + rightText
	}

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

	profilesStr := "none"
	if len(tr.Profiles) > 0 {
		profilesStr = strings.Join(tr.Profiles, " -> ")
	}

	divider := strings.Repeat("─", contentWidth)

	// Section 1: Header
	lines := []string{
		formatHeaderLine("Target:", tr.Target),
		formatHeaderLine("Profiles:", profilesStr),
		formatHeaderLine("Mode:", tr.Mode),
		"",
	}

	// Section 2: Statistics
	lines = append(lines,
		"Progress",
		divider,
		formatStatsRow("Elapsed:", elapsedStr, "ETA:", etaStr),
		formatStatsRow("Requests:", fmt.Sprintf("%d", snap.RequestsSent), "Responses:", fmt.Sprintf("%d", snap.ResponsesReceived)),
		formatStatsRow("Req/s:", fmt.Sprintf("%.0f", snap.RequestsPerSecond), "Workers:", fmt.Sprintf("%d", snap.ActiveWorkers)),
		formatStatsRow("Queue:", fmt.Sprintf("%d", snap.QueuedJobs), "Found:", fmt.Sprintf("%d", snap.Discovered)),
		formatStatsRow("Filtered:", fmt.Sprintf("%d", snap.RequestsFiltered), "Failed:", fmt.Sprintf("%d", snap.RequestsFailed)),
		formatStatsRow("Avg Latency:", formatLatency(snap.AverageLatency), "Retries/Redir:", fmt.Sprintf("%d / %d", snap.Retries, snap.Redirects)),
		"",
	)

	// Section 3: Recent discoveries
	lines = append(lines,
		"Recent discoveries",
		divider,
	)

	for _, l := range lines {
		fmt.Fprintf(tr.Writer, "\033[K%s%s\n", paddingStr, l)
	}

	// Output recent discoveries section
	for i := 0; i < tr.limit; i++ {
		if i < len(tr.recent) {
			entry := tr.recent[i]
			statusPrefix := fmt.Sprintf("%d  ", entry.statusCode)
			maxPathLen := contentWidth - len(statusPrefix)
			p := entry.path
			if len(p) > maxPathLen && maxPathLen > 3 {
				p = p[:maxPathLen-3] + "..."
			}
			line := statusPrefix + p
			fmt.Fprintf(tr.Writer, "\033[K%s%s\n", paddingStr, line)
		} else {
			fmt.Fprintf(tr.Writer, "\033[K%s\n", paddingStr)
		}
	}

	// Section 4: Controls
	fmt.Fprintf(tr.Writer, "\033[K%s\n", paddingStr)
	fmt.Fprintf(tr.Writer, "\033[K%sControls:\n", paddingStr)
	fmt.Fprintf(tr.Writer, "\033[K%s[p] refresh    [s] statistics    [q] graceful stop\n", paddingStr)

	if tr.frozen {
		// Restore cursor
		fmt.Fprint(tr.Writer, "\033[u")
	}

	return nil
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
