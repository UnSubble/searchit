package state_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/state"
)

func TestPhase_String_AllPhases(t *testing.T) {
	phases := []struct {
		phase state.Phase
		want  string
	}{
		{state.PhaseStarting, "STARTING"},
		{state.PhaseRunning, "RUNNING"},
		{state.PhasePaused, "PAUSED"},
		{state.PhaseShutdownRequested, "SHUTDOWN REQUESTED"},
		{state.PhaseStopping, "GRACEFUL STOPPING"},
		{state.PhaseDraining, "DRAINING"},
		{state.PhaseWaitingWorkers, "WAITING WORKERS"},
		{state.PhaseFinalizing, "FINALIZING"},
		{state.PhaseTerminalShutdown, "TERMINAL_SHUTDOWN"},
		{state.PhaseSummary, "SUMMARY"},
		{state.PhasePipeline, "PIPELINE"},
		{state.PhaseDone, "DONE"},
		{state.Phase(999), "UNKNOWN(999)"},
	}

	for _, tc := range phases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.phase.String(); got != tc.want {
				t.Errorf("Phase(%d).String() = %q, want %q", int(tc.phase), got, tc.want)
			}
		})
	}
}

func TestStateMachineTransitions(t *testing.T) {
	sm := state.NewManager()
	if sm.Current() != state.PhaseStarting {
		t.Fatalf("expected initial phase STARTING, got %s", sm.Current())
	}

	var transitions []string
	sm.OnTransition(func(oldPhase, newPhase state.Phase) {
		transitions = append(transitions, oldPhase.String()+"->"+newPhase.String())
	})

	// No-op transition should not trigger listeners
	sm.Transition(state.PhaseStarting)
	if len(transitions) != 0 {
		t.Fatalf("expected 0 transitions on identical phase, got %d", len(transitions))
	}

	sm.Transition(state.PhaseRunning)
	if sm.Current() != state.PhaseRunning {
		t.Errorf("expected phase RUNNING, got %s", sm.Current())
	}

	sm.Transition(state.PhaseStopping)
	sm.Transition(state.PhaseWaitingWorkers)
	sm.Transition(state.PhaseFinalizing)
	sm.Transition(state.PhaseTerminalShutdown)
	sm.Transition(state.PhaseSummary)
	sm.Transition(state.PhasePipeline)
	sm.Transition(state.PhaseDone)

	if sm.Current() != state.PhaseDone {
		t.Errorf("expected final phase DONE, got %s", sm.Current())
	}

	expectedTransitions := []string{
		"STARTING->RUNNING",
		"RUNNING->GRACEFUL STOPPING",
		"GRACEFUL STOPPING->WAITING WORKERS",
		"WAITING WORKERS->FINALIZING",
		"FINALIZING->TERMINAL_SHUTDOWN",
		"TERMINAL_SHUTDOWN->SUMMARY",
		"SUMMARY->PIPELINE",
		"PIPELINE->DONE",
	}

	if len(transitions) != len(expectedTransitions) {
		t.Fatalf("expected %d transitions, got %d:\n%s", len(expectedTransitions), len(transitions), strings.Join(transitions, "\n"))
	}

	for i, tr := range transitions {
		if tr != expectedTransitions[i] {
			t.Errorf("transition %d mismatch: got %s, want %s", i, tr, expectedTransitions[i])
		}
	}
}

func TestStateMachine_WaitUntilRunning_Immediate(t *testing.T) {
	sm := state.NewManager()
	sm.Transition(state.PhaseRunning)

	err := sm.WaitUntilRunning(context.Background())
	if err != nil {
		t.Fatalf("expected nil error when already running, got %v", err)
	}
}

func TestStateMachine_WaitUntilRunning_PauseAndResume(t *testing.T) {
	sm := state.NewManager()
	sm.Transition(state.PhaseRunning)
	sm.Transition(state.PhasePaused)

	var wg sync.WaitGroup
	wg.Add(1)

	var waitErr error
	go func() {
		defer wg.Done()
		waitErr = sm.WaitUntilRunning(context.Background())
	}()

	time.Sleep(20 * time.Millisecond)
	sm.Transition(state.PhaseRunning) // resume

	wg.Wait()
	if waitErr != nil {
		t.Fatalf("expected nil error after resume, got %v", waitErr)
	}
}

func TestStateMachine_WaitUntilRunning_ContextCancel(t *testing.T) {
	sm := state.NewManager()
	sm.Transition(state.PhasePaused)

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)

	var waitErr error
	go func() {
		defer wg.Done()
		waitErr = sm.WaitUntilRunning(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	wg.Wait()
	if !errors.Is(waitErr, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", waitErr)
	}
}
