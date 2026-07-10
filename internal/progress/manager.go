package progress

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/unsubble/searchit/internal/console"
	"github.com/unsubble/searchit/internal/stats"
)

// Manager handles periodic rendering of runtime statistics.
type Manager struct {
	Collector *stats.Collector
	Renderer  Renderer
	Interval  time.Duration
}

// NewManager creates a new progress Manager.
func NewManager(collector *stats.Collector, renderer Renderer, interval time.Duration) *Manager {
	if interval <= 0 {
		interval = 1 * time.Second
	}
	return &Manager{
		Collector: collector,
		Renderer:  renderer,
		Interval:  interval,
	}
}

// Start launches the periodic refresh loop. Blocks until the context is cancelled.
// It also listens for user commands from cmdChan if provided.
func (m *Manager) Start(ctx context.Context, cmdChan <-chan console.Command) {
	ticker := time.NewTicker(m.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final render of statistics
			_ = m.Renderer.Render(m.Collector.Snapshot())
			return
		case <-ticker.C:
			_ = m.Renderer.Render(m.Collector.Snapshot())
		case cmd, ok := <-cmdChan:
			if !ok {
				cmdChan = nil
				break
			}
			switch cmd {
			case console.CommandProgress:
				_ = m.Renderer.Render(m.Collector.Snapshot())
			case console.CommandStats:
				m.PrintStats()
			}
		}
	}
}

// PrintStats prints an extended statistics block.
func (m *Manager) PrintStats() {
	var w io.Writer = os.Stdout
	if tr, ok := m.Renderer.(*ANSIRenderer); ok {
		tr.Clear()
		w = tr.Writer
	} else if tr, ok := m.Renderer.(*TextRenderer); ok {
		w = tr.Writer
	}

	snap := m.Collector.Snapshot()
	elapsed := time.Since(snap.StartTime)
	h := int(elapsed.Hours())
	mMin := int(elapsed.Minutes()) % 60
	s := int(elapsed.Seconds()) % 60
	elapsedStr := fmt.Sprintf("%02d:%02d:%02d", h, mMin, s)

	fmt.Fprintln(w, "\n--- Extended Statistics ---")
	fmt.Fprintf(w, "Elapsed:             %s\n", elapsedStr)
	fmt.Fprintf(w, "Requests:            %d\n", snap.RequestsSent)
	fmt.Fprintf(w, "Responses:           %d\n", snap.ResponsesReceived)
	fmt.Fprintf(w, "Discovered:          %d\n", snap.Discovered)
	fmt.Fprintf(w, "Filtered:            %d\n", snap.RequestsFiltered)
	fmt.Fprintf(w, "Failed:              %d\n", snap.RequestsFailed)
	fmt.Fprintf(w, "Bytes:               %d\n", snap.BytesReceived)
	fmt.Fprintf(w, "Workers:             %d\n", snap.ActiveWorkers)
	fmt.Fprintf(w, "Queue:               %d\n", snap.QueuedJobs)
	fmt.Fprintf(w, "Req/s:               %.0f\n", snap.RequestsPerSecond)
	fmt.Fprintln(w, "Status distribution:")

	var codes []int
	for c := range snap.StatusCodes {
		codes = append(codes, c)
	}
	for i := 0; i < len(codes); i++ {
		for j := i + 1; j < len(codes); j++ {
			if codes[i] > codes[j] {
				codes[i], codes[j] = codes[j], codes[i]
			}
		}
	}
	for _, c := range codes {
		fmt.Fprintf(w, "  %d: %d\n", c, snap.StatusCodes[c])
	}
	fmt.Fprintln(w, "---------------------------")

	if _, ok := m.Renderer.(*ANSIRenderer); ok {
		_ = m.Renderer.Render(snap)
	}
}

// RecordResult feeds a discovered url into the renderer if it supports results registration.
func (m *Manager) RecordResult(statusCode int, urlStr string) {
	if tr, ok := m.Renderer.(*ANSIRenderer); ok {
		tr.AddResult(statusCode, urlStr)
	}
}
