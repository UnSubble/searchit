package progress

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/unsubble/searchit/internal/stats"
)

// TextRenderer renders statistics snapshots as clean plain text output.
type TextRenderer struct {
	Writer io.Writer
	Target string
}

// NewTextRenderer instantiates a new TextRenderer writing to the provided Writer.
func NewTextRenderer(w io.Writer, target string) *TextRenderer {
	if w == nil {
		w = os.Stdout
	}
	return &TextRenderer{
		Writer: w,
		Target: target,
	}
}

// Render writes the snapshot to the target writer.
func (tr *TextRenderer) Render(snap stats.Snapshot) error {
	elapsed := time.Since(snap.StartTime)
	h := int(elapsed.Hours())
	m := int(elapsed.Minutes()) % 60
	s := int(elapsed.Seconds()) % 60
	elapsedStr := fmt.Sprintf("%02d:%02d:%02d", h, m, s)

	fmt.Fprintf(tr.Writer, "Target:      %s\n", tr.Target)
	fmt.Fprintf(tr.Writer, "Requests:    %d\n", snap.RequestsSent)
	fmt.Fprintf(tr.Writer, "Responses:   %d\n", snap.ResponsesReceived)
	fmt.Fprintf(tr.Writer, "Workers:     %d\n", snap.ActiveWorkers)
	fmt.Fprintf(tr.Writer, "Queue:       %d\n", snap.QueuedJobs)
	fmt.Fprintf(tr.Writer, "Discovered:  %d\n", snap.Discovered)
	fmt.Fprintf(tr.Writer, "Req/s:       %.0f\n", snap.RequestsPerSecond)
	fmt.Fprintf(tr.Writer, "Elapsed:     %s\n\n", elapsedStr)

	return nil
}
