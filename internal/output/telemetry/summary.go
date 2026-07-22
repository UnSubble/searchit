package telemetry

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/unsubble/searchit/internal/output"
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

func PrintSummaryWithWidth(w io.Writer, info SummaryInfo, debugMode bool, width int) {
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

	items := []output.Item{
		{Key: "Candidates", Value: strconv.Itoa(info.Candidates)},
		{Key: "Findings", Value: strconv.Itoa(info.Findings)},
		{Key: "Wall Time", Value: fmt.Sprintf("%.2f sec", wallTimeSec)},
		{Key: "Req/sec", Value: strconv.FormatInt(reqPerSec, 10)},
	}

	if debugMode {
		adaptiveStr := "disabled"
		if info.AdaptiveEnabled {
			adaptiveStr = "enabled"
		}
		items = append(items,
			output.Item{Key: "Strategy", Value: info.Strategy},
			output.Item{Key: "Adaptive", Value: adaptiveStr},
			output.Item{Key: "Requests Sent", Value: strconv.FormatInt(info.Snapshot.RequestsSent, 10)},
			output.Item{Key: "Requests Completed", Value: strconv.FormatInt(info.Snapshot.ResponsesReceived, 10)},
		)

		var codes []int
		for c := range info.Snapshot.StatusCodes {
			codes = append(codes, c)
		}
		sort.Ints(codes)
		for _, c := range codes {
			count := info.Snapshot.StatusCodes[c]
			items = append(items, output.Item{
				Key:   fmt.Sprintf("Status %d", c),
				Value: strconv.FormatInt(count, 10),
			})
		}
		items = append(items, output.Item{Key: "Requests Filtered", Value: strconv.FormatInt(info.Snapshot.RequestsFiltered, 10)})
	}

	output.RenderBlock(w, title, items, width)
}

func PrintSummary(w io.Writer, info SummaryInfo, debugMode bool) {
	PrintSummaryWithWidth(w, info, debugMode, 0)
}
