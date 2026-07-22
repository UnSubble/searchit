package state

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Phase represents an explicit step in the application execution state machine.
type Phase int32

const (
	PhaseStarting Phase = iota
	PhaseRunning
	PhasePaused
	PhaseStopping
	PhaseWaitingWorkers
	PhaseFinalizing
	PhaseSummary
	PhasePipeline
	PhaseDone
)

func (p Phase) String() string {
	switch p {
	case PhaseStarting:
		return "STARTING"
	case PhaseRunning:
		return "RUNNING"
	case PhasePaused:
		return "PAUSED"
	case PhaseStopping:
		return "GRACEFUL STOPPING"
	case PhaseWaitingWorkers:
		return "WAITING WORKERS"
	case PhaseFinalizing:
		return "FINALIZING"
	case PhaseSummary:
		return "SUMMARY"
	case PhasePipeline:
		return "PIPELINE"
	case PhaseDone:
		return "DONE"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", p)
	}
}

// Manager is a thread-safe execution phase state machine.
type Manager struct {
	current   int32
	mu        sync.Mutex
	listeners []func(oldPhase, newPhase Phase)
}

// NewManager initializes a state machine starting in PhaseStarting.
func NewManager() *Manager {
	return &Manager{
		current: int32(PhaseStarting),
	}
}

// Current returns the current execution phase.
func (m *Manager) Current() Phase {
	return Phase(atomic.LoadInt32(&m.current))
}

// Transition moves the state machine to a new phase if valid.
func (m *Manager) Transition(to Phase) Phase {
	m.mu.Lock()
	defer m.mu.Unlock()

	old := Phase(atomic.LoadInt32(&m.current))
	if old == to {
		return old
	}

	atomic.StoreInt32(&m.current, int32(to))

	for _, listener := range m.listeners {
		listener(old, to)
	}

	return to
}

// OnTransition registers a callback invoked when phase transitions occur.
func (m *Manager) OnTransition(fn func(oldPhase, newPhase Phase)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, fn)
}
