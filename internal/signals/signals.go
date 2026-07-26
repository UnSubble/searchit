package signals

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// SetupGlobal registers a global OS signal handler for the process lifetime.
// It invokes onGraceful when the first SIGINT/SIGTERM is caught.
// It invokes onForce if a second signal is caught.
// It returns when ctx is done.
func SetupGlobal(ctx context.Context, onGraceful func(), onForce func()) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		defer signal.Stop(sigChan)

		select {
		case <-sigChan:
			if onGraceful != nil {
				onGraceful()
			}
		case <-ctx.Done():
			return
		}

		select {
		case <-sigChan:
			if onForce != nil {
				onForce()
			}
		case <-ctx.Done():
			return
		}

		// Keep eating signals until context is natively finished.
		for {
			select {
			case <-sigChan:
			case <-ctx.Done():
				return
			}
		}
	}()
}
