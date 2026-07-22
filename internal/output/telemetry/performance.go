package telemetry

import (
	"fmt"
	"io"
	"time"

	"github.com/unsubble/searchit/internal/output/terminal"
)

type PerformanceInfo struct {
	StartTime    time.Time
	RequestsSent int64
}

func GetPerformanceItems(info PerformanceInfo) []terminal.Item {
	elapsed := time.Since(info.StartTime)
	wallTimeSec := elapsed.Seconds()

	var reqPerSec int64
	if wallTimeSec > 0 {
		reqPerSec = int64(float64(info.RequestsSent) / wallTimeSec)
	}

	return []terminal.Item{
		{Key: "Wall Time", Value: fmt.Sprintf("%.2f sec", wallTimeSec)},
		{Key: "Req/sec", Value: fmt.Sprintf("%d", reqPerSec)},
	}
}

func PrintPerformance(w io.Writer, info PerformanceInfo) {
	items := GetPerformanceItems(info)
	for _, item := range items {
		fmt.Fprintln(w, terminal.FormatDotRow(item.Key, item.Value, terminal.DefaultPadWidth, 0))
	}
}
