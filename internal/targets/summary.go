package targets

import (
	"sync"
	"time"

	"github.com/unsubble/searchit/internal/stats"
)

// GlobalSummary aggregates statistics across multiple target executions.
type GlobalSummary struct {
	mu           sync.Mutex
	StartTime    time.Time
	TargetsTotal int
	TargetsRun   int
	TotalJobs    int64
	TotalFound   int64
	TotalErrors  int64
	// Extensible for more global metrics
}

// NewGlobalSummary creates a new summary collector.
func NewGlobalSummary(totalTargets int) *GlobalSummary {
	return &GlobalSummary{
		StartTime:    time.Now(),
		TargetsTotal: totalTargets,
	}
}

// AddSnapshot merges a per-target snapshot into the global summary.
func (s *GlobalSummary) AddSnapshot(snap stats.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TargetsRun++
	s.TotalJobs += snap.RequestsSent
	s.TotalFound += snap.Discovered
}

// Duration returns the total time elapsed since creation.
func (s *GlobalSummary) Duration() time.Duration {
	return time.Since(s.StartTime)
}
