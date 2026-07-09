package progress

import (
	"context"
	"time"

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
func (m *Manager) Start(ctx context.Context) {
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
		}
	}
}
