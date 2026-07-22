package state_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/state"
)

func TestStateMachineTransitions(t *testing.T) {
	sm := state.NewManager()
	if sm.Current() != state.PhaseStarting {
		t.Fatalf("expected initial phase STARTING, got %s", sm.Current())
	}

	var transitions []string
	sm.OnTransition(func(oldPhase, newPhase state.Phase) {
		transitions = append(transitions, oldPhase.String()+"->"+newPhase.String())
	})

	sm.Transition(state.PhaseRunning)
	if sm.Current() != state.PhaseRunning {
		t.Errorf("expected phase RUNNING, got %s", sm.Current())
	}

	sm.Transition(state.PhaseStopping)
	sm.Transition(state.PhaseWaitingWorkers)
	sm.Transition(state.PhaseFinalizing)
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
		"FINALIZING->SUMMARY",
		"SUMMARY->PIPELINE",
		"PIPELINE->DONE",
	}

	if len(transitions) != len(expectedTransitions) {
		t.Fatalf("expected %d transitions, got %d", len(expectedTransitions), len(transitions))
	}

	for i, tr := range transitions {
		if tr != expectedTransitions[i] {
			t.Errorf("transition %d mismatch: got %s, want %s", i, tr, expectedTransitions[i])
		}
	}
}
