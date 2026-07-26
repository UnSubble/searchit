package telemetry

import (
	"fmt"
	"io"
	"time"

	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/presentation"
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
		{Key: "Wall Time", Value: presentation.Duration(elapsed)},
		{Key: "Req/sec", Value: presentation.Number(reqPerSec)},
	}
}

func PrintPerformance(w io.Writer, info PerformanceInfo) {
	items := GetPerformanceItems(info)
	for _, item := range items {
		fmt.Fprintf(w, "%-28s %s\n", item.Key, item.Value)
	}
}
