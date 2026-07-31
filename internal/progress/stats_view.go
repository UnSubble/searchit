package progress

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/unsubble/searchit/internal/presentation"
	"github.com/unsubble/searchit/internal/stats"
)

// statsReport builds the compact, left-aligned inspection statistics overlay as a []string, one entry per line.
// It is a pure formatter: no cursor movement, no column math, no width assumptions.
func statsReport(
	contentWidth int,
	snap stats.Snapshot,
	configuredThreads int,
	target string,
	profiles []string,
	mode string,
	method string,
	httpVersion string,
) []string {
	var lines []string

	add := func(s string) { lines = append(lines, s) }
	formatRow := func(label string, value string) string {
		return fmt.Sprintf("%-18s : %s", label, value)
	}

	if method == "" {
		method = "GET"
	}
	if httpVersion == "" {
		httpVersion = "HTTP/1.1"
	}

	add("Statistics (press any key to return)")
	add("")
	add(formatRow("Method", strings.ToUpper(method)))
	add(formatRow("HTTP", httpVersion))
	add("")
	add(formatRow("Requests Sent", presentation.Number(snap.RequestsSent)))
	add(formatRow("Findings", presentation.Number(snap.Discovered)))
	add(formatRow("Errors", presentation.Number(snap.RequestsFailed)))
	add(formatRow("Retries", presentation.Number(snap.Retries)))
	add("")
	add(formatRow("Bytes Received", presentation.Number(snap.BytesReceived)))
	add(formatRow("Total Candidates", presentation.Number(snap.TotalCandidates)))
	add("")
	add(formatRow("Total Req/sec", fmt.Sprintf("%.0f", snap.CurrentRequestsPerSecond)))
	add(formatRow("Elapsed", presentation.Duration(time.Since(snap.StartTime))))

	return lines
}

// RenderStatsView clears the terminal, prints the sequential statistics report,
// and returns. It does not position the cursor or assume any terminal dimensions.
func RenderStatsView(w io.Writer, snap stats.Snapshot, configuredThreads int) {
	RenderStatsViewFull(w, 80, snap, configuredThreads, "", nil, "", "", "")
}

// RenderStatsViewFull renders the compact statistics view to w.
func RenderStatsViewFull(
	w io.Writer,
	contentWidth int,
	snap stats.Snapshot,
	configuredThreads int,
	target string,
	profiles []string,
	mode string,
	method string,
	httpVersion string,
) {
	lines := statsReport(contentWidth, snap, configuredThreads, target, profiles, mode, method, httpVersion)
	for i, line := range lines {
		if i > 0 {
			fmt.Fprint(w, "\r\n")
		}
		fmt.Fprintf(w, "\033[K%s", line)
	}
}
