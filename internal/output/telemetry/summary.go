package telemetry

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/stats"
)

type SummaryInfo struct {
	IsFuzz          bool
	Strategy        string
	AdaptiveEnabled bool
	Findings        int
	Snapshot        stats.Snapshot
}

// PrintSummary prints the end-of-scan summary block.
// All output is routed through tm.Emit(owner, fn).
func PrintSummary(tm *terminal.Manager, owner terminal.Owner, info SummaryInfo, debugMode bool) {
	title := "SCAN SUMMARY"
	if info.IsFuzz {
		title = "FUZZ SUMMARY"
	}

	elapsed := time.Since(info.Snapshot.StartTime)
	wallTimeSec := elapsed.Seconds()
	var reqPerSec int64
	if wallTimeSec > 0 {
		reqPerSec = int64(float64(info.Snapshot.RequestsSent) / wallTimeSec)
	}

	candidates := int(info.Snapshot.JobsProduced)
	if candidates == 0 && info.IsFuzz {
		candidates = int(info.Snapshot.RequestsSent)
	}

	invalidWords := int(info.Snapshot.InvalidWords)

	items := []terminal.Item{
		{Key: "Candidates", Value: strconv.Itoa(candidates)},
		{Key: "Findings", Value: strconv.Itoa(info.Findings)},
		{Key: "Wall Time", Value: fmt.Sprintf("%.2f sec", wallTimeSec)},
		{Key: "Req/sec", Value: strconv.FormatInt(reqPerSec, 10)},
	}
	if invalidWords > 0 {
		items = append(items, terminal.Item{Key: "Invalid Words", Value: strconv.Itoa(invalidWords)})
	}

	if debugMode {
		adaptiveStr := "disabled"
		if info.AdaptiveEnabled {
			adaptiveStr = "enabled"
		}
		items = append(items,
			terminal.Item{Key: "Strategy", Value: info.Strategy},
			terminal.Item{Key: "Adaptive", Value: adaptiveStr},
			terminal.Item{Key: "Requests Sent", Value: strconv.FormatInt(info.Snapshot.RequestsSent, 10)},
			terminal.Item{Key: "Requests Completed", Value: strconv.FormatInt(info.Snapshot.ResponsesReceived, 10)},
		)

		var codes []int
		for c := range info.Snapshot.StatusCodes {
			codes = append(codes, c)
		}
		sort.Ints(codes)
		for _, c := range codes {
			count := info.Snapshot.StatusCodes[c]
			items = append(items, terminal.Item{
				Key:   fmt.Sprintf("Status %d", c),
				Value: strconv.FormatInt(count, 10),
			})
		}
		items = append(items, terminal.Item{Key: "Requests Filtered", Value: strconv.FormatInt(info.Snapshot.RequestsFiltered, 10)})
	}

	_ = tm.Emit(owner, func(w io.Writer) {
		terminal.RenderBlock(w, title, items, tm.ContentWidth())
	})
}
