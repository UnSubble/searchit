package signals

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/unsubble/searchit/internal/state"
)

// SetupContext wraps parentCtx in a signal-notified context listening for SIGINT and SIGTERM.
// When a signal is caught, it updates stateMgr to PhaseStopping and cancels the context cleanly.
func SetupContext(parentCtx context.Context, stateMgr *state.Manager) (context.Context, context.CancelFunc) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	ctx, cancel := context.WithCancel(parentCtx)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-sigChan:
			if stateMgr != nil && stateMgr.Current() < state.PhaseStopping {
				stateMgr.Transition(state.PhaseStopping)
			}
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sigChan)
	}()

	return ctx, cancel
}
