package terminal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
)

// ──────────────────────────────────────────────────────────────────────────────
// Errors
// ──────────────────────────────────────────────────────────────────────────────

var (
	// ErrInvalidTransition is returned when a phase transition is not permitted.
	ErrInvalidTransition = errors.New("terminal: invalid phase transition")

	// ErrWrongOwner is returned when Emit or ReleaseOwner is called by the wrong owner.
	ErrWrongOwner = errors.New("terminal: wrong owner")

	// ErrOwnerConflict is returned when AcquireOwner is called while another owner is active.
	ErrOwnerConflict = errors.New("terminal: owner conflict")
)

// ──────────────────────────────────────────────────────────────────────────────
// Phase — lifecycle state machine
// ──────────────────────────────────────────────────────────────────────────────

// Phase represents a step in the execution lifecycle.
// Transitions are strictly validated: only forward moves in the declared table
// are permitted. Invalid transitions panic in tests and return an error in
// production, making it impossible to silently reach a forbidden state.
type Phase int

const (
	PhaseStarting         Phase = iota // initial; config may be printed
	PhaseRunning                       // scan/fuzz active; progress renders
	PhaseStopping                      // SIGINT / graceful stop requested
	PhaseWaitingWorkers                // draining in-flight requests
	PhaseFinalizing                    // final cleanup; no output
	PhaseTerminalShutdown              // explicit terminal teardown before summary
	PhaseSummary                       // end-of-scan summary block
	PhasePipeline                      // pipeline reconciliation block (debug)
	PhaseDone                          // terminal; no further output
)

var phaseNames = [...]string{
	PhaseStarting:         "STARTING",
	PhaseRunning:          "RUNNING",
	PhaseStopping:         "STOPPING",
	PhaseWaitingWorkers:   "WAITING_WORKERS",
	PhaseFinalizing:       "FINALIZING",
	PhaseTerminalShutdown: "TERMINAL_SHUTDOWN",
	PhaseSummary:          "SUMMARY",
	PhasePipeline:         "PIPELINE",
	PhaseDone:             "DONE",
}

func (p Phase) String() string {
	if int(p) >= 0 && int(p) < len(phaseNames) {
		return phaseNames[p]
	}
	return fmt.Sprintf("Phase(%d)", int(p))
}

// validTransitions is the ONLY set of allowed state moves.
// Any transition not in this table is illegal.
var validTransitions = [...]uint32{
	// Encode allowed destinations as a bitmask over Phase values (0–8).
	PhaseStarting:         1<<PhaseRunning | 1<<PhaseStopping,
	PhaseRunning:          1<<PhaseStopping | 1<<PhaseWaitingWorkers,
	PhaseStopping:         1<<PhaseWaitingWorkers | 1<<PhaseDone,
	PhaseWaitingWorkers:   1 << PhaseFinalizing,
	PhaseFinalizing:       1<<PhaseTerminalShutdown | 1<<PhaseDone,
	PhaseTerminalShutdown: 1<<PhaseSummary | 1<<PhaseDone,
	PhaseSummary:          1<<PhasePipeline | 1<<PhaseDone,
	PhasePipeline:         1 << PhaseDone,
	PhaseDone:             0, // terminal — no exits
}

func isValidTransition(from, to Phase) bool {
	if int(from) >= len(validTransitions) {
		return false
	}
	return validTransitions[from]&(1<<to) != 0
}

// ──────────────────────────────────────────────────────────────────────────────
// Owner — who may write to stdout right now
// ──────────────────────────────────────────────────────────────────────────────

// Owner identifies which subsystem currently holds the terminal.
// Only ONE owner may exist at any time.
type Owner int

const (
	OwnerNone          Owner = iota // stdout is unowned; no writes allowed
	OwnerConfiguration              // configuration block is being printed
	OwnerProgress                   // live progress dashboard is active
	OwnerStatistics                 // full-screen statistics view is active
	OwnerSummary                    // end-of-scan summary block
	OwnerPipeline                   // pipeline reconciliation block
)

var ownerNames = [...]string{
	OwnerNone:          "None",
	OwnerConfiguration: "Configuration",
	OwnerProgress:      "Progress",
	OwnerStatistics:    "Statistics",
	OwnerSummary:       "Summary",
	OwnerPipeline:      "Pipeline",
}

func (o Owner) String() string {
	if int(o) >= 0 && int(o) < len(ownerNames) {
		return ownerNames[o]
	}
	return fmt.Sprintf("Owner(%d)", int(o))
}

// phaseWriteAllowed returns true if any writes are permitted in the given phase.
// PhaseWaitingWorkers, PhaseFinalizing, and PhaseDone are silent — no subsystem
// may write during these transitions.
func phaseWriteAllowed(p Phase) bool {
	switch p {
	case PhaseWaitingWorkers, PhaseFinalizing, PhaseDone:
		return false
	}
	return true
}

// ──────────────────────────────────────────────────────────────────────────────
// Manager — the single owner of stdout
// ──────────────────────────────────────────────────────────────────────────────

// Manager is the single, process-wide owner of stdout.
//
// All terminal writes MUST go through Manager.Emit(). This is enforced by:
//  1. Making the underlying writer a private field — nothing else can obtain it.
//  2. Protecting every write with a single sync.Mutex — concurrent writes are
//     mathematically impossible: at most one goroutine executes inside Emit at
//     any time.
//  3. Validating the caller's Owner token before every write — a subsystem
//     that does not currently hold ownership cannot write.
//  4. Validating phase transitions against a strict, forward-only table —
//     invalid moves panic in test mode and return errors in production.
//
// Design contract:
//   - STARTING  → owner=Configuration (config block)
//   - RUNNING   → owner=Progress or Statistics (live dashboard)
//   - STOPPING  → owner=Progress (final render before drain)
//   - WAITING_WORKERS / FINALIZING → owner=None (silent)
//   - SUMMARY   → owner=Summary
//   - PIPELINE  → owner=Pipeline
//   - DONE      → owner=None, no further writes
type Manager struct {
	mu    sync.Mutex // THE single global output lock
	out   io.Writer  // private — nothing else holds this reference
	phase Phase
	owner Owner
}

// New creates a Manager writing to w. Pass os.Stdout for production.
// In tests, pass a *bytes.Buffer or similar for isolation.
func New(w io.Writer) *Manager {
	if w == nil {
		w = os.Stdout
	}
	return &Manager{out: w}
}

// Stop completely terminates the state machine.
// Usually called automatically by context cancellation.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.phase = PhaseDone
	m.owner = OwnerNone
}

// Width returns the true terminal column count for the managed stdout.
// This is the single source of truth for all layout components.
func (m *Manager) Width() int {
	return width(m.out)
}

// ContentWidth returns the constrained width suitable for content rendering.
func (m *Manager) ContentWidth() int {
	return contentWidth(m.out)
}

// Phase returns the current lifecycle phase. Safe to call from any goroutine.
func (m *Manager) CurrentPhase() Phase {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.phase
}

// CurrentOwner returns the current stdout owner. Safe to call from any goroutine.
func (m *Manager) CurrentOwner() Owner {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.owner
}

// ─── Phase transitions ────────────────────────────────────────────────────────

// TransitionTo advances the lifecycle state machine.
//
// If the transition is invalid:
//   - In test mode: panics immediately.
//   - In production: returns ErrInvalidTransition (no state change).
func (m *Manager) TransitionTo(to Phase) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.transitionLocked(to)
}

// transitionLocked performs the transition while m.mu is already held.
func (m *Manager) transitionLocked(to Phase) error {
	if !isValidTransition(m.phase, to) {
		msg := fmt.Sprintf("terminal: invalid phase transition %v → %v", m.phase, to)
		if testing.Testing() {
			panic(msg)
		}
		return fmt.Errorf("%w: %s", ErrInvalidTransition, msg)
	}
	m.phase = to
	return nil
}

// ─── Ownership ────────────────────────────────────────────────────────────────

// AcquireOwner claims stdout for the given subsystem.
//
// Fails (panic in tests / error in prod) if:
//   - Another subsystem already owns stdout.
//   - The current phase is PhaseDone (terminal state).
func (m *Manager) AcquireOwner(o Owner) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.acquireLocked(o)
}

func (m *Manager) acquireLocked(o Owner) error {
	if m.phase == PhaseDone {
		msg := fmt.Sprintf("terminal: cannot acquire owner %v: phase is DONE", o)
		if testing.Testing() {
			panic(msg)
		}
		return fmt.Errorf("%w: %s", ErrOwnerConflict, msg)
	}
	if m.owner != OwnerNone && m.owner != o {
		msg := fmt.Sprintf("terminal: cannot acquire owner %v: currently owned by %v", o, m.owner)
		if testing.Testing() {
			panic(msg)
		}
		return fmt.Errorf("%w: %s", ErrOwnerConflict, msg)
	}
	m.owner = o
	return nil
}

// ReleaseOwner relinquishes stdout ownership, setting it back to OwnerNone.
//
// Panics in tests (error in prod) if o is not the current owner.
func (m *Manager) ReleaseOwner(o Owner) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.releaseLocked(o)
}

func (m *Manager) releaseLocked(o Owner) error {
	if m.owner != o {
		msg := fmt.Sprintf("terminal: cannot release owner %v: currently owned by %v", o, m.owner)
		if testing.Testing() {
			panic(msg)
		}
		return fmt.Errorf("%w: %s", ErrWrongOwner, msg)
	}
	m.owner = OwnerNone
	return nil
}

// ─── Atomic helpers ───────────────────────────────────────────────────────────

// AcquireAndTransition atomically transitions the phase and claims ownership.
// Use this when a phase change and ownership acquisition must be indivisible.
func (m *Manager) AcquireAndTransition(o Owner, to Phase) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.transitionLocked(to); err != nil {
		return err
	}
	return m.acquireLocked(o)
}

// TransitionAndRelease atomically transitions the phase and releases ownership.
// Use this when ending a rendering phase.
func (m *Manager) TransitionAndRelease(to Phase, o Owner) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.transitionLocked(to); err != nil {
		return err
	}
	return m.releaseLocked(o)
}

// ─── The single write path ────────────────────────────────────────────────────

// Emit is the ONLY method that may write to stdout.
//
// It acquires the global output lock, validates the caller's Owner token,
// validates that writes are permitted in the current phase, then calls fn
// with the underlying writer.
//
// Mathematical guarantee: sync.Mutex ensures at most one goroutine executes
// inside fn at any time. Combined with the requirement that all writes must go
// through Emit, concurrent stdout writes are impossible by construction.
//
// Panics in test mode if:
//   - o does not match the current owner.
//   - The current phase does not permit writes (WaitingWorkers, Finalizing, Done).
func (m *Manager) Emit(o Owner, fn func(io.Writer)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.owner != o {
		msg := fmt.Sprintf("terminal: Emit owner mismatch: current=%v attempted=%v (phase=%v)",
			m.owner, o, m.phase)
		if testing.Testing() {
			panic(msg)
		}
		return fmt.Errorf("%w: %s", ErrWrongOwner, msg)
	}

	if !phaseWriteAllowed(m.phase) {
		msg := fmt.Sprintf("terminal: Emit not permitted in phase %v", m.phase)
		if testing.Testing() {
			panic(msg)
		}
		return fmt.Errorf("%w: %s", ErrWrongOwner, msg)
	}

	fn(m.out)
	return nil
}

// ─── Owner-switch helpers (used by statistics view) ──────────────────────────

// SwitchOwner atomically transfers ownership from old to new.
// Used by the progress manager when switching between Progress and Statistics.
func (m *Manager) SwitchOwner(from, to Owner) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.owner != from {
		msg := fmt.Sprintf("terminal: SwitchOwner: expected %v, got %v", from, m.owner)
		if testing.Testing() {
			panic(msg)
		}
		return fmt.Errorf("%w: %s", ErrWrongOwner, msg)
	}
	m.owner = to
	return nil
}
