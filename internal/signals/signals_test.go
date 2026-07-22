package signals_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/signals"
	"github.com/unsubble/searchit/internal/state"
)

func TestSignalSetupContext_NormalCancel(t *testing.T) {
	sm := state.NewManager()
	ctx, cancel := signals.SetupContext(context.Background(), sm)

	if sm.Current() != state.PhaseStarting {
		t.Errorf("expected phase STARTING, got %s", sm.Current())
	}

	cancel()
	<-ctx.Done()

	time.Sleep(10 * time.Millisecond)

	// Manual cancel should not set phase to stopping
	if sm.Current() == state.PhaseStopping {
		t.Errorf("manual cancel set state to STOPPING unexpectedly")
	}
}

func TestSignalSetupContext_SignalDelivery(t *testing.T) {
	sm := state.NewManager()
	ctx, cancel := signals.SetupContext(context.Background(), sm)
	defer cancel()

	// Simulate OS interrupt signal delivery to self (cross-platform Linux/macOS/Windows)
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("failed to find process: %v", err)
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		t.Skipf("skipping signal test: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatalf("context was not cancelled after interrupt signal")
	}

	// Give signal listener goroutine a moment to transition state
	deadline := time.Now().Add(1 * time.Second)
	for sm.Current() != state.PhaseStopping && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if sm.Current() != state.PhaseStopping {
		t.Errorf("expected phase STOPPING after interrupt signal, got %s", sm.Current())
	}
}
