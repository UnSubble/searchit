package progress

import "github.com/unsubble/searchit/internal/stats"

// Renderer defines the contract for rendering runtime statistics snapshots.
// Implementation must be stateless.
type Renderer interface {
	Render(stats.Snapshot) error
}
