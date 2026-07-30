package progress

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/presentation"
	"github.com/unsubble/searchit/internal/stats"
)

// statsReport builds the complete statistics report as a []string, one entry per line.
// It is a pure formatter: no cursor movement, no column math, no width assumptions.
func statsReport(
	contentWidth int,
	snap stats.Snapshot,
	configuredThreads int,
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

	subtitle := func(name string) string {
		return "  " + name
	}

	formatRow := func(label, value string) string {
		return "  " + fmt.Sprintf("%-20s %s", label, value)
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
	add(formatRow("Search Space", presentation.Number(snap.TotalCandidates)))
	add(formatRow("Generated Jobs", presentation.Number(snap.JobsProduced)))
	add(formatRow("Findings", presentation.Number(snap.Discovered)))
	add(formatRow("Requests sent", presentation.Number(snap.RequestsSent)))
	add(formatRow("Responses received", presentation.Number(snap.ResponsesReceived)))
	add(formatRow("Successful", presentation.Number(snap.RequestsSucceeded)))
	add(formatRow("Filtered", presentation.Number(snap.RequestsFiltered)))
	add(formatRow("Failed", presentation.Number(snap.RequestsFailed)))
	add(formatRow("Bytes received", presentation.Number(snap.BytesReceived)))
	add(formatRow("Redirects", presentation.Number(snap.Redirects)))
	add(formatRow("Retries", presentation.Number(snap.Retries)))
	add("")
	add(subtitle("PERFORMANCE"))
	add("")
	add(formatRow("Total requests", presentation.Number(snap.RequestsSent)))
	add(formatRow("Current Req/s", fmt.Sprintf("%.0f", snap.CurrentRequestsPerSecond)))
	add(formatRow("Average Req/s", fmt.Sprintf("%.0f", snap.RequestsPerSecond)))
	add(formatRow("Peak Req/s", fmt.Sprintf("%.0f", snap.PeakRequestsPerSecond)))
	add("")
	add(subtitle("TIMING"))
	add("")
	add(formatRow("Elapsed", presentation.Duration(time.Since(snap.StartTime))))
	add(formatRow("Average latency", terminal.FormatLatency(snap.AverageLatency)))

	// ── Workers ─────────────────────────────────────────────────────────────
	section("Workers")
	add(formatRow("Configured", fmt.Sprintf("%d", configuredThreads)))
	add(formatRow("Active", fmt.Sprintf("%d", snap.ActiveWorkers)))

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
			add(fmt.Sprintf("  %-6d  %s", c, presentation.Number(snap.StatusCodes[c])))
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
func RenderStatsView(w io.Writer, snap stats.Snapshot, configuredThreads int) {
	RenderStatsViewFull(w, 80, snap, configuredThreads, "", nil, "")
}

// RenderStatsViewFull renders the complete statistics view to w.
func RenderStatsViewFull(
	w io.Writer,
	contentWidth int,
	snap stats.Snapshot,
	configuredThreads int,
	target string,
	profiles []string,
	mode string,
) {
	fmt.Fprint(w, "\033[H\033[2J")
	lines := statsReport(contentWidth, snap, configuredThreads, target, profiles, mode)
	for _, line := range lines {
		fmt.Fprintf(w, "\r%s\r\n", line)
	}
}
