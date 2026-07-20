package telemetry

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/unsubble/searchit/internal/stats"
)

type SummaryInfo struct {
	IsFuzz          bool
	Strategy        string
	AdaptiveEnabled bool
	Candidates      int
	Findings        int
	Snapshot        stats.Snapshot
}

func padDots(label string) string {
	targetWidth := 20
	if len(label) >= targetWidth {
		return label + " "
	}
	dotsCount := targetWidth - len(label) - 1
	if dotsCount < 0 {
		dotsCount = 0
	}
	return label + " " + strings.Repeat(".", dotsCount) + " "
}

func PrintSummary(w io.Writer, info SummaryInfo, debugMode bool) {
	fmt.Fprintln(w, "---------------------------------------------------------")
	fmt.Fprintln(w)
	if info.IsFuzz {
		fmt.Fprintln(w, "FUZZ SUMMARY")
	} else {
		fmt.Fprintln(w, "SCAN SUMMARY")
	}
	fmt.Fprintln(w)

	if !debugMode {
		fmt.Fprintf(w, "Candidates:\n    %d\n\n", info.Candidates)
		fmt.Fprintf(w, "Findings:\n    %d\n\n", info.Findings)
		PrintPerformance(w, PerformanceInfo{
			StartTime:    info.Snapshot.StartTime,
			RequestsSent: info.Snapshot.RequestsSent,
		})
		fmt.Fprintln(w, "---------------------------------------------------------")
		return
	}

	fmt.Fprintf(w, "Strategy:\n    %s\n\n", info.Strategy)

	adaptiveStr := "disabled"
	if info.AdaptiveEnabled {
		adaptiveStr = "enabled"
	}
	fmt.Fprintf(w, "Adaptive:\n    %s\n\n", adaptiveStr)

	fmt.Fprintf(w, "Candidates:\n    %d\n\n", info.Candidates)
	fmt.Fprintf(w, "Findings:\n    %d\n\n", info.Findings)

	fmt.Fprintln(w, "Requests:")
	fmt.Fprintf(w, "    %s%d\n", padDots("Sent"), info.Snapshot.RequestsSent)
	fmt.Fprintf(w, "    %s%d\n\n", padDots("Completed"), info.Snapshot.ResponsesReceived)

	fmt.Fprintln(w, "Responses:")
	// Sort status codes numerically
	var codes []int
	for c := range info.Snapshot.StatusCodes {
		codes = append(codes, c)
	}
	sort.Ints(codes)
	for _, c := range codes {
		count := info.Snapshot.StatusCodes[c]
		fmt.Fprintf(w, "    %s%d\n", padDots(strconv.Itoa(c)), count)
	}
	fmt.Fprintf(w, "    %s%d\n\n", padDots("Filtered"), info.Snapshot.RequestsFiltered)

	PrintPerformance(w, PerformanceInfo{
		StartTime:    info.Snapshot.StartTime,
		RequestsSent: info.Snapshot.RequestsSent,
	})
	fmt.Fprintln(w, "---------------------------------------------------------")
}
