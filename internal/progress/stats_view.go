package progress

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/stats"
)

// statsReport builds the complete statistics report as a []string, one entry per line.
// It is a pure formatter: no cursor movement, no column math, no width assumptions.
func statsReport(
	contentWidth int,
	snap stats.Snapshot,
	configuredThreads int,
	recent []discoveryEntry,
	target string,
	profiles []string,
	mode string,
) []string {
	var lines []string

	thinDivider := strings.Repeat(terminal.ThinSeparatorChar, contentWidth)
	thickDivider := strings.Repeat(terminal.ThickSeparatorChar, contentWidth)

	add := func(s string) { lines = append(lines, s) }
	blank := func() { lines = append(lines, "") }
	section := func(name string) {
		blank()
		add(name)
		add(thinDivider)
		blank()
	}

	formatRow := func(label, value string) string {
		return "  " + terminal.FormatDotRow(label, value, 22, contentWidth-2)
	}

	// ── Header ──────────────────────────────────────────────────────────────
	add("Searchit Statistics")
	add(thickDivider)

	// ── Scan context ────────────────────────────────────────────────────────
	section("Target")
	add("  " + target)

	section("Profiles")
	if len(profiles) == 0 {
		add("  none")
	} else {
		add("  " + strings.Join(profiles, " -> "))
	}

	section("Mode")
	add("  " + mode)

	// ── General ─────────────────────────────────────────────────────────────
	section("General")
	add(formatRow("Requests sent", formatNumber(snap.RequestsSent)))
	add(formatRow("Responses received", formatNumber(snap.ResponsesReceived)))
	add(formatRow("Successful", formatNumber(snap.RequestsSucceeded)))
	add(formatRow("Filtered", formatNumber(snap.RequestsFiltered)))
	add(formatRow("Failed", formatNumber(snap.RequestsFailed)))
	add(formatRow("Bytes received", formatNumber(snap.BytesReceived)))
	add(formatRow("Redirects", formatNumber(snap.Redirects)))
	add(formatRow("Retries", formatNumber(snap.Retries)))

	// ── Performance ─────────────────────────────────────────────────────────
	section("Performance")
	add(formatRow("Elapsed", terminal.FormatElapsed(time.Since(snap.StartTime))))
	add(formatRow("Average latency", terminal.FormatLatency(snap.AverageLatency)))
	add(formatRow("Requests/sec", fmt.Sprintf("%.0f", snap.RequestsPerSecond)))
	add(formatRow("Peak requests/sec", fmt.Sprintf("%.0f", snap.PeakRequestsPerSecond)))

	// ── Workers ─────────────────────────────────────────────────────────────
	section("Workers")
	add(formatRow("Configured", fmt.Sprintf("%d", configuredThreads)))
	add(formatRow("Active", fmt.Sprintf("%d", snap.ActiveWorkers)))
	add(formatRow("Queue", fmt.Sprintf("%d", snap.QueuedJobs)))

	// ── Response Codes ──────────────────────────────────────────────────────
	section("Response Codes")
	var codes []int
	for c := range snap.StatusCodes {
		if snap.StatusCodes[c] > 0 {
			codes = append(codes, c)
		}
	}
	sort.Ints(codes)
	if len(codes) == 0 {
		add("  No responses received yet.")
	} else {
		for _, c := range codes {
			add(fmt.Sprintf("  %-6d  %s", c, formatNumber(snap.StatusCodes[c])))
		}
	}

	// ── Recent discoveries ──────────────────────────────────────────────────
	section("Recent discoveries")
	if len(recent) == 0 {
		add("  No discoveries yet.")
	} else {
		for _, entry := range recent {
			add(fmt.Sprintf("  %d  %s", entry.statusCode, entry.path))
		}
	}

	// ── Footer ───────────────────────────────────────────────────────────────
	blank()
	add(thickDivider)
	blank()
	add("Press any key to return...")

	return lines
}

// RenderStatsView clears the terminal, prints the sequential statistics report,
// and returns. It does not position the cursor or assume any terminal dimensions.
func RenderStatsView(w io.Writer, snap stats.Snapshot, configuredThreads int, recent []discoveryEntry) {
	RenderStatsViewFull(w, 80, snap, configuredThreads, recent, "", nil, "")
}

// RenderStatsViewFull renders the complete statistics view to w.
func RenderStatsViewFull(
	w io.Writer,
	contentWidth int,
	snap stats.Snapshot,
	configuredThreads int,
	recent []discoveryEntry,
	target string,
	profiles []string,
	mode string,
) {
	fmt.Fprint(w, "\033[H\033[2J")
	lines := statsReport(contentWidth, snap, configuredThreads, recent, target, profiles, mode)
	for _, line := range lines {
		fmt.Fprintf(w, "\r%s\r\n", line)
	}
}

func formatNumber(n int64) string {
	if n < 0 {
		return fmt.Sprintf("%d", n)
	}
	in := fmt.Sprintf("%d", n)
	if len(in) <= 3 {
		return in
	}
	out := make([]byte, len(in)+(len(in)-1)/3)
	for i, j, k := len(in)-1, len(out)-1, 0; i >= 0; i-- {
		out[j] = in[i]
		j--
		k++
		if k == 3 && i > 0 {
			out[j] = ','
			j--
			k = 0
		}
	}
	return string(out)
}
