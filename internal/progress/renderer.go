package progress

import "github.com/unsubble/searchit/internal/stats"

// Renderer defines the contract for rendering runtime statistics snapshots.
// All implementations must route their output through the TerminalManager.
type Renderer interface {
	Render(stats.Snapshot) error
	// AddResult records a discovered URL for display in the recent-discoveries list.
	AddResult(statusCode int, urlStr string)
}
