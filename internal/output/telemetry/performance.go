package telemetry

import (
	"fmt"
	"io"
	"time"
)

type PerformanceInfo struct {
	StartTime    time.Time
	RequestsSent int64
}

func PrintPerformance(w io.Writer, info PerformanceInfo) {
	elapsed := time.Since(info.StartTime)
	wallTimeSec := elapsed.Seconds()

	var reqPerSec int64
	if wallTimeSec > 0 {
		reqPerSec = int64(float64(info.RequestsSent) / wallTimeSec)
	}

	fmt.Fprintln(w, "Performance:")
	fmt.Fprintf(w, "    %s%.2f sec\n", padDots("Wall Time"), wallTimeSec)
	fmt.Fprintf(w, "    %s%d\n", padDots("Req/sec"), reqPerSec)
	fmt.Fprintln(w)
}
