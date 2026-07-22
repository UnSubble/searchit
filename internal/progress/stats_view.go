package progress

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/unsubble/searchit/internal/stats"
)

const (
	reportThinDivider  = "──────────────────────────────────────────────────────────────"
	reportThickDivider = "══════════════════════════════════════════════════════════════"
)

// statsReport builds the complete statistics report as a []string, one entry per line.
// It is a pure formatter: no cursor movement, no column math, no width assumptions.
func statsReport(
	snap stats.Snapshot,
	configuredThreads int,
	recent []discoveryEntry,
	target string,
	profiles []string,
	mode string,
) []string {
	var lines []string

	add := func(s string) { lines = append(lines, s) }
	blank := func() { lines = append(lines, "") }
	section := func(name string) {
		blank()
		add(name)
		add(reportThinDivider)
		blank()
	}

	// ── Header ──────────────────────────────────────────────────────────────
	add("Searchit Statistics")
	add(reportThickDivider)

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
	add(fmt.Sprintf("  %-22s  %s", "Requests sent", formatNumber(snap.RequestsSent)))
	add(fmt.Sprintf("  %-22s  %s", "Responses received", formatNumber(snap.ResponsesReceived)))
	add(fmt.Sprintf("  %-22s  %s", "Successful", formatNumber(snap.RequestsSucceeded)))
	add(fmt.Sprintf("  %-22s  %s", "Filtered", formatNumber(snap.RequestsFiltered)))
	add(fmt.Sprintf("  %-22s  %s", "Failed", formatNumber(snap.RequestsFailed)))
	add(fmt.Sprintf("  %-22s  %s", "Bytes received", formatNumber(snap.BytesReceived)))
	add(fmt.Sprintf("  %-22s  %s", "Redirects", formatNumber(snap.Redirects)))
	add(fmt.Sprintf("  %-22s  %s", "Retries", formatNumber(snap.Retries)))

	// ── Performance ─────────────────────────────────────────────────────────
	elapsed := time.Since(snap.StartTime)
	h := int(elapsed.Hours())
	m := int(elapsed.Minutes()) % 60
	s := int(elapsed.Seconds()) % 60
	elapsedStr := fmt.Sprintf("%02d:%02d:%02d", h, m, s)

	section("Performance")
	add(fmt.Sprintf("  %-22s  %s", "Elapsed", elapsedStr))
	add(fmt.Sprintf("  %-22s  %s", "Average latency", formatLatency(snap.AverageLatency)))
	add(fmt.Sprintf("  %-22s  %.0f", "Requests/sec", snap.RequestsPerSecond))
	add(fmt.Sprintf("  %-22s  %.0f", "Peak requests/sec", snap.PeakRequestsPerSecond))

	// ── Workers ─────────────────────────────────────────────────────────────
	section("Workers")
	add(fmt.Sprintf("  %-22s  %d", "Configured", configuredThreads))
	add(fmt.Sprintf("  %-22s  %d", "Active", snap.ActiveWorkers))
	add(fmt.Sprintf("  %-22s  %d", "Queue", snap.QueuedJobs))

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
	add(reportThickDivider)
	blank()
	add("Press any key to return...")

	return lines
}

// RenderStatsView clears the terminal, prints the sequential statistics report,
// and returns. It does not position the cursor or assume any terminal dimensions.
func RenderStatsView(w io.Writer, snap stats.Snapshot, configuredThreads int, recent []discoveryEntry) {
	RenderStatsViewFull(w, snap, configuredThreads, recent, "", nil, "")
}

// RenderStatsViewFull is the full-featured entry point that includes scan context
// (target, profiles, mode) in the report header.
func RenderStatsViewFull(
	w io.Writer,
	snap stats.Snapshot,
	configuredThreads int,
	recent []discoveryEntry,
	target string,
	profiles []string,
	mode string,
) {
	lines := statsReport(snap, configuredThreads, recent, target, profiles, mode)
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
