package targets

import (
	"context"
)

// JobFunc is the signature for the execution closure over a single target.
type JobFunc func(tCtx TargetContext) error

// Manager coordinates the execution lifecycle of multiple targets.
type Manager struct {
	targets []Target
}

// NewManager initializes a new target execution manager.
func NewManager(targets []Target) *Manager {
	return &Manager{
		targets: targets,
	}
}

// Execute runs the provided JobFunc over all targets sequentially.
// It manages target-level cancellation (e.g., 'q') versus global abortion (e.g., 'a' or SIGINT).
func (m *Manager) Execute(globalCtx context.Context, job JobFunc) error {
	for i := range m.targets {
		t := &m.targets[i]

		// Check if global context is cancelled before starting next target.
		if globalCtx.Err() != nil {
			return globalCtx.Err()
		}

		// Create an isolated context for this specific target.
		targetCtx, cancelTarget := context.WithCancel(globalCtx)

		tCtx := TargetContext{
			Ctx:    targetCtx,
			Cancel: cancelTarget,
			Target: t,
		}

		err := job(tCtx)

		// Always ensure cleanup.
		cancelTarget()

		// If the error indicates a fatal global abort or if global context is done,
		// we break out of the target loop entirely.
		if err != nil {
			// A specific SkipTarget error could be used to gracefully continue,
			// but for now we'll assume any non-context-cancellation error might be fatal,
			// or we can just log it and continue. Let's just return it if it's fatal.
			// Currently searchit aborts on execution setup errors.

			// If it's a context cancellation, we need to check if it's global or local.
			if err == context.Canceled {
				if globalCtx.Err() != nil {
					// Global abort
					return globalCtx.Err()
				}
				// Local skip via 'q', continue to next target.
				continue
			}

			// If we get here, it's a real error (e.g., network setup failure).
			return err
		}
	}
	return nil
}
