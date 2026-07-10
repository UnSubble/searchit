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
)

// ANSIRenderer renders statistics snapshots using live ANSI updates in the terminal.
type ANSIRenderer struct {
	Writer   io.Writer
	Target   string
	Profiles []string
	Mode     string
	limit    int

	mu     sync.Mutex
	recent []string
	frozen bool
}

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
		// Reserve space for 23 lines
		for i := 0; i < 23; i++ {
			fmt.Fprintln(w)
		}
		// Set scroll margin to start at line 24
		fmt.Fprint(w, "\033[24;r")
		// Move cursor to start of scroll margin
		fmt.Fprint(w, "\033[24;1H")
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
	entry := fmt.Sprintf("%d  %s", statusCode, path)

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
		for i := 0; i < 23; i++ {
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

	linesToPrint := []string{
		fmt.Sprintf("Target:        %s", tr.Target),
		fmt.Sprintf("Profiles:      %s", profilesStr),
		fmt.Sprintf("Mode:          %s", tr.Mode),
		"",
		"Progress",
		"──────────────────────────────────────────────",
		fmt.Sprintf("Elapsed:       %-10s         ETA:           %-10s", elapsedStr, etaStr),
		fmt.Sprintf("Requests:      %-10d         Responses:     %-10d", snap.RequestsSent, snap.ResponsesReceived),
		fmt.Sprintf("Req/s:         %-10.0f         Workers:       %-10d", snap.RequestsPerSecond, snap.ActiveWorkers),
		fmt.Sprintf("Queue:         %-10d         Found:         %-10d", snap.QueuedJobs, snap.Discovered),
		fmt.Sprintf("Filtered:      %-10d         Failed:        %-10d", snap.RequestsFiltered, snap.RequestsFailed),
		fmt.Sprintf("Avg Latency:   %-10s         Retries/Redir: %d / %d", formatLatency(snap.AverageLatency), snap.Retries, snap.Redirects),
		"",
		"Recent discoveries",
		"──────────────────────────────────────────────",
	}

	for _, l := range linesToPrint {
		fmt.Fprintf(tr.Writer, "\033[K%s\n", l)
	}

	for i := 0; i < tr.limit; i++ {
		if i < len(tr.recent) {
			fmt.Fprintf(tr.Writer, "\033[K%s\n", tr.recent[i])
		} else {
			fmt.Fprintf(tr.Writer, "\033[K\n")
		}
	}

	fmt.Fprintf(tr.Writer, "\033[K\n")
	fmt.Fprintf(tr.Writer, "\033[KControls:\n")
	fmt.Fprintf(tr.Writer, "\033[K[p] refresh    [s] statistics    [q] graceful stop\n")

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
