package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/unsubble/searchit/internal/stats"
)

// ANSIRenderer renders statistics snapshots using live ANSI updates in the terminal.
type ANSIRenderer struct {
	Writer io.Writer
	Target string
	limit  int

	mu     sync.Mutex
	recent []string
	lines  int
}

// NewANSIRenderer creates a new ANSIRenderer writing to the provided Writer.
// Automatically hides terminal cursor to prevent flickers.
func NewANSIRenderer(w io.Writer, target string) *ANSIRenderer {
	if w == nil {
		w = os.Stdout
	}
	// Hide cursor: \033[?25l
	fmt.Fprint(w, "\033[?25l")
	return &ANSIRenderer{
		Writer: w,
		Target: target,
		limit:  10,
	}
}

// AddResult records a successful discovery path.
func (tr *ANSIRenderer) AddResult(statusCode int, urlStr string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	path := extractPath(tr.Target, urlStr)
	entry := fmt.Sprintf("%d  %s", statusCode, path)

	if len(tr.recent) >= tr.limit {
		tr.recent = append(tr.recent[1:], entry)
	} else {
		tr.recent = append(tr.recent, entry)
	}
}

// Close restores the terminal state by showing the cursor.
func (tr *ANSIRenderer) Close() error {
	// Show cursor: \033[?25h
	fmt.Fprint(tr.Writer, "\033[?25h")
	return nil
}

// Clear erases the previously printed block using ANSI escape sequences.
func (tr *ANSIRenderer) Clear() {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if tr.lines > 0 {
		fmt.Fprintf(tr.Writer, "\033[%dA", tr.lines)
		for i := 0; i < tr.lines; i++ {
			fmt.Fprint(tr.Writer, "\033[K\n")
		}
		fmt.Fprintf(tr.Writer, "\033[%dA", tr.lines)
		tr.lines = 0
	}
}

// Render overwrites the previously printed block using ANSI escape sequences.
func (tr *ANSIRenderer) Render(snap stats.Snapshot) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	// Move cursor up by tr.lines
	if tr.lines > 0 {
		fmt.Fprintf(tr.Writer, "\033[%dA", tr.lines)
	}

	elapsed := time.Since(snap.StartTime)
	h := int(elapsed.Hours())
	m := int(elapsed.Minutes()) % 60
	s := int(elapsed.Seconds()) % 60
	elapsedStr := fmt.Sprintf("%02d:%02d:%02d", h, m, s)

	var printed int

	linesToPrint := []string{
		"Scanning:",
		tr.Target,
		"",
		"Progress",
		"",
		fmt.Sprintf("Elapsed      %s", elapsedStr),
		fmt.Sprintf("Requests     %d", snap.RequestsSent),
		fmt.Sprintf("Responses    %d", snap.ResponsesReceived),
		fmt.Sprintf("Discovered   %d", snap.Discovered),
		fmt.Sprintf("Workers      %d", snap.ActiveWorkers),
		fmt.Sprintf("Queue        %d", snap.QueuedJobs),
		fmt.Sprintf("Rate         %.0f req/s", snap.RequestsPerSecond),
		"",
		"Recent discoveries",
		"",
	}

	for _, l := range linesToPrint {
		fmt.Fprintf(tr.Writer, "\033[K%s\n", l)
		printed++
	}

	for i := 0; i < tr.limit; i++ {
		if i < len(tr.recent) {
			fmt.Fprintf(tr.Writer, "\033[K%s\n", tr.recent[i])
		} else {
			fmt.Fprintf(tr.Writer, "\033[K\n")
		}
		printed++
	}

	tr.lines = printed
	return nil
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
