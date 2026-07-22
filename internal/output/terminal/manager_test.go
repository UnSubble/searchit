package terminal_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	terminal "github.com/unsubble/searchit/internal/output/terminal"
)

// ──────────────────────────────────────────────────────────────────────────────
// Phase transition tests
// ──────────────────────────────────────────────────────────────────────────────

func TestPhaseTransition_ValidLifecycle_Scan(t *testing.T) {
	m := terminal.New(io.Discard)
	steps := []terminal.Phase{
		terminal.PhaseRunning,
		terminal.PhaseWaitingWorkers,
		terminal.PhaseFinalizing,
		terminal.PhaseTerminalShutdown,
		terminal.PhaseSummary,
		terminal.PhasePipeline,
		terminal.PhaseDone,
	}
	for _, to := range steps {
		if err := m.TransitionTo(to); err != nil {
			t.Fatalf("unexpected error transitioning to %v: %v", to, err)
		}
	}
}

func TestPhaseTransition_ValidLifecycle_FuzzWithStop(t *testing.T) {
	m := terminal.New(io.Discard)
	steps := []terminal.Phase{
		terminal.PhaseRunning,
		terminal.PhaseStopping,
		terminal.PhaseWaitingWorkers,
		terminal.PhaseFinalizing,
		terminal.PhaseTerminalShutdown,
		terminal.PhaseSummary,
		terminal.PhaseDone,
	}
	for _, to := range steps {
		if err := m.TransitionTo(to); err != nil {
			t.Fatalf("unexpected error transitioning to %v: %v", to, err)
		}
	}
}

func TestPhaseTransition_InvalidSkipForward(t *testing.T) {
	// Starting → Summary is not allowed (must go through Running first).
	m := terminal.New(io.Discard)
	assertPanics(t, "STARTING → SUMMARY must panic", func() {
		_ = m.TransitionTo(terminal.PhaseSummary)
	})
}

func TestPhaseTransition_InvalidBackward_SummaryToRunning(t *testing.T) {
	m := advanceTo(t, terminal.PhaseSummary)
	assertPanics(t, "SUMMARY → RUNNING must panic", func() {
		_ = m.TransitionTo(terminal.PhaseRunning)
	})
}

func TestPhaseTransition_InvalidBackward_DoneToRunning(t *testing.T) {
	m := advanceTo(t, terminal.PhaseDone)
	assertPanics(t, "DONE → RUNNING must panic", func() {
		_ = m.TransitionTo(terminal.PhaseRunning)
	})
}

func TestPhaseTransition_InvalidBackward_PipelineToProgress(t *testing.T) {
	m := advanceTo(t, terminal.PhasePipeline)
	// No "progress" phase per se, but try PIPELINE → RUNNING.
	assertPanics(t, "PIPELINE → RUNNING must panic", func() {
		_ = m.TransitionTo(terminal.PhaseRunning)
	})
}

func TestPhaseTransition_DoneIsTerminal(t *testing.T) {
	m := advanceTo(t, terminal.PhaseDone)
	for _, p := range []terminal.Phase{
		terminal.PhaseStarting,
		terminal.PhaseRunning,
		terminal.PhaseStopping,
		terminal.PhaseWaitingWorkers,
		terminal.PhaseFinalizing,
		terminal.PhaseSummary,
		terminal.PhasePipeline,
	} {
		assertPanics(t, fmt.Sprintf("DONE → %v must panic", p), func() {
			_ = m.TransitionTo(p)
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Owner tests
// ──────────────────────────────────────────────────────────────────────────────

func TestOwner_AcquireAndRelease(t *testing.T) {
	m := terminal.New(io.Discard)
	if err := m.AcquireOwner(terminal.OwnerConfiguration); err != nil {
		t.Fatalf("AcquireOwner: %v", err)
	}
	if m.CurrentOwner() != terminal.OwnerConfiguration {
		t.Fatalf("expected OwnerConfiguration, got %v", m.CurrentOwner())
	}
	if err := m.ReleaseOwner(terminal.OwnerConfiguration); err != nil {
		t.Fatalf("ReleaseOwner: %v", err)
	}
	if m.CurrentOwner() != terminal.OwnerNone {
		t.Fatalf("expected OwnerNone after release, got %v", m.CurrentOwner())
	}
}

func TestOwner_ConflictPanics(t *testing.T) {
	m := terminal.New(io.Discard)
	if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}
	assertPanics(t, "acquiring Summary while Progress owns must panic", func() {
		_ = m.AcquireOwner(terminal.OwnerSummary)
	})
}

func TestOwner_WrongReleasePanics(t *testing.T) {
	m := terminal.New(io.Discard)
	if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}
	assertPanics(t, "releasing Summary when Progress owns must panic", func() {
		_ = m.ReleaseOwner(terminal.OwnerSummary)
	})
}

func TestOwner_CannotAcquireOnDone(t *testing.T) {
	m := advanceTo(t, terminal.PhaseDone)
	assertPanics(t, "AcquireOwner on DONE must panic", func() {
		_ = m.AcquireOwner(terminal.OwnerSummary)
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Emit tests
// ──────────────────────────────────────────────────────────────────────────────

func TestEmit_WritesToUnderlying(t *testing.T) {
	var buf bytes.Buffer
	m := terminal.New(&buf)
	if err := m.AcquireOwner(terminal.OwnerConfiguration); err != nil {
		t.Fatal(err)
	}
	if err := m.Emit(terminal.OwnerConfiguration, func(w io.Writer) {
		fmt.Fprint(w, "hello")
	}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello" {
		t.Fatalf("expected 'hello', got %q", buf.String())
	}
}

func TestEmit_WrongOwnerPanics(t *testing.T) {
	m := terminal.New(io.Discard)
	if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}
	assertPanics(t, "Emit with wrong owner must panic", func() {
		_ = m.Emit(terminal.OwnerSummary, func(w io.Writer) {
			fmt.Fprint(w, "should not reach here")
		})
	})
}

func TestEmit_RejectedAfterRelease(t *testing.T) {
	m := terminal.New(io.Discard)
	if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}
	if err := m.ReleaseOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}
	// Now owner=None; Emit(OwnerProgress) must panic.
	assertPanics(t, "Emit after Release must panic", func() {
		_ = m.Emit(terminal.OwnerProgress, func(w io.Writer) {
			fmt.Fprint(w, "should not reach here")
		})
	})
}

func TestEmit_RejectedInSilentPhase(t *testing.T) {
	// Advance a manager to WaitingWorkers while someone holds Progress owner.
	m := terminal.New(io.Discard)
	mustTransition(t, m, terminal.PhaseRunning)
	if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}
	// Simulate: progress goroutine stopped, ownership released, phase advanced.
	if err := m.ReleaseOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}
	mustTransition(t, m, terminal.PhaseWaitingWorkers)
	// Now owner=None, phase=WaitingWorkers.
	// AcquireOwner must succeed (we don't block AcquireOwner in silent phases,
	// but Emit must be rejected).
	if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}
	assertPanics(t, "Emit in WaitingWorkers must panic", func() {
		_ = m.Emit(terminal.OwnerProgress, func(w io.Writer) { fmt.Fprint(w, "x") })
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Concurrency tests — prove simultaneous writes are impossible
// ──────────────────────────────────────────────────────────────────────────────

// TestConcurrentEmit_NoCorruption launches N goroutines all writing to the same
// Manager. The race detector must report zero races, and the output must be
// exactly N atomic writes (no interleaving).
func TestConcurrentEmit_NoCorruption(t *testing.T) {
	const N = 100
	var buf bytes.Buffer
	m := terminal.New(&buf)
	if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}
	// Advance to Running so writes are permitted.
	if err := m.TransitionTo(terminal.PhaseRunning); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = m.Emit(terminal.OwnerProgress, func(w io.Writer) {
				fmt.Fprintf(w, "goroutine%d\n", id)
			})
		}(i)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != N {
		t.Fatalf("expected %d lines (atomic writes), got %d", N, len(lines))
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "goroutine") {
			t.Errorf("line corrupted (partial write detected): %q", line)
		}
	}
}

// TestProgressAndResultConcurrent simulates the progress ticker and HandleResult
// running simultaneously — both going through Emit(OwnerProgress).
// The race detector must find zero races.
func TestProgressAndResultConcurrent(t *testing.T) {
	const iters = 1000
	var buf bytes.Buffer
	m := terminal.New(&buf)
	if err := m.TransitionTo(terminal.PhaseRunning); err != nil {
		t.Fatal(err)
	}
	if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup

	// Simulate progress ticker.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			_ = m.Emit(terminal.OwnerProgress, func(w io.Writer) {
				fmt.Fprint(w, "P")
			})
		}
	}()

	// Simulate result callback.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			_ = m.Emit(terminal.OwnerProgress, func(w io.Writer) {
				fmt.Fprint(w, "R")
			})
		}
	}()

	wg.Wait()

	got := buf.String()
	// Every character must be either P or R — no corruption.
	for i, c := range got {
		if c != 'P' && c != 'R' {
			t.Fatalf("output corruption at index %d: %q", i, c)
		}
	}
	if len(got) != 2*iters {
		t.Fatalf("expected %d characters, got %d", 2*iters, len(got))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Ownership handoff — progress → summary must be sequential
// ──────────────────────────────────────────────────────────────────────────────

func TestProgressToSummaryHandoff(t *testing.T) {
	var buf bytes.Buffer
	m := terminal.New(&buf)

	// === RUNNING phase: Progress owns stdout ===
	if err := m.TransitionTo(terminal.PhaseRunning); err != nil {
		t.Fatal(err)
	}
	if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}

	// Progress writes.
	_ = m.Emit(terminal.OwnerProgress, func(w io.Writer) { fmt.Fprint(w, "[progress]") })

	// Summary must NOT be able to write while Progress owns.
	assertPanics(t, "Summary Emit while Progress owns must panic", func() {
		_ = m.Emit(terminal.OwnerSummary, func(w io.Writer) { fmt.Fprint(w, "[summary]") })
	})

	// === FINALIZING: release Progress, advance phase ===
	if err := m.ReleaseOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}
	mustTransition(t, m, terminal.PhaseWaitingWorkers)
	mustTransition(t, m, terminal.PhaseFinalizing)
	mustTransition(t, m, terminal.PhaseTerminalShutdown)
	mustTransition(t, m, terminal.PhaseSummary)

	// Progress must NOT be able to emit now (owner=None, Summary phase).
	// Verify by acquiring Progress and checking Emit is rejected due to wrong owner.
	// (We can't Emit without acquiring, so demonstrate the ownership mismatch path.)

	// Summary can now write.
	if err := m.AcquireOwner(terminal.OwnerSummary); err != nil {
		t.Fatal(err)
	}
	// Attempting Progress emit while Summary owns must panic.
	assertPanics(t, "Emit(Progress) while Summary owns must panic", func() {
		_ = m.Emit(terminal.OwnerProgress, func(w io.Writer) { fmt.Fprint(w, "bad") })
	})
	_ = m.Emit(terminal.OwnerSummary, func(w io.Writer) { fmt.Fprint(w, "[summary-ok]") })

	if !strings.Contains(buf.String(), "[progress]") {
		t.Error("progress output missing")
	}
	if !strings.Contains(buf.String(), "[summary-ok]") {
		t.Error("summary output missing")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Multi-target: fresh Manager per target makes DONE truly terminal
// ──────────────────────────────────────────────────────────────────────────────

func TestMultiTarget_FreshManagerPerTarget(t *testing.T) {
	var allOutput bytes.Buffer

	for i := 0; i < 3; i++ {
		// Each target gets its own Manager — PhaseDone is terminal for that target.
		m := terminal.New(&allOutput)
		mustTransition(t, m, terminal.PhaseRunning)
		if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
			t.Fatal(err)
		}
		_ = m.Emit(terminal.OwnerProgress, func(w io.Writer) {
			fmt.Fprintf(w, "[progress-%d]", i)
		})
		if err := m.ReleaseOwner(terminal.OwnerProgress); err != nil {
			t.Fatal(err)
		}
		mustTransition(t, m, terminal.PhaseWaitingWorkers)
		mustTransition(t, m, terminal.PhaseFinalizing)
		mustTransition(t, m, terminal.PhaseTerminalShutdown)
		mustTransition(t, m, terminal.PhaseSummary)
		if err := m.AcquireOwner(terminal.OwnerSummary); err != nil {
			t.Fatal(err)
		}
		_ = m.Emit(terminal.OwnerSummary, func(w io.Writer) {
			fmt.Fprintf(w, "[summary-%d]", i)
		})
		if err := m.ReleaseOwner(terminal.OwnerSummary); err != nil {
			t.Fatal(err)
		}
		mustTransition(t, m, terminal.PhaseDone)

		// After PhaseDone, no transitions allowed on this Manager.
		assertPanics(t, fmt.Sprintf("target %d: transition after Done must panic", i), func() {
			_ = m.TransitionTo(terminal.PhaseRunning)
		})
	}

	got := allOutput.String()
	for i := 0; i < 3; i++ {
		if !strings.Contains(got, fmt.Sprintf("[progress-%d]", i)) {
			t.Errorf("missing progress output for target %d", i)
		}
		if !strings.Contains(got, fmt.Sprintf("[summary-%d]", i)) {
			t.Errorf("missing summary output for target %d", i)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// SwitchOwner — Progress ↔ Statistics
// ──────────────────────────────────────────────────────────────────────────────

func TestSwitchOwner_ProgressToStatistics(t *testing.T) {
	m := terminal.New(io.Discard)
	mustTransition(t, m, terminal.PhaseRunning)
	if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}

	// Switch to Statistics.
	if err := m.SwitchOwner(terminal.OwnerProgress, terminal.OwnerStatistics); err != nil {
		t.Fatalf("SwitchOwner: %v", err)
	}

	// Progress must not write now.
	assertPanics(t, "Emit(Progress) after switch to Statistics must panic", func() {
		_ = m.Emit(terminal.OwnerProgress, func(w io.Writer) {})
	})

	// Statistics can write.
	if err := m.Emit(terminal.OwnerStatistics, func(w io.Writer) {}); err != nil {
		t.Fatalf("Emit(Statistics): %v", err)
	}

	// Switch back to Progress.
	if err := m.SwitchOwner(terminal.OwnerStatistics, terminal.OwnerProgress); err != nil {
		t.Fatalf("SwitchOwner back: %v", err)
	}

	// Progress can write again.
	if err := m.Emit(terminal.OwnerProgress, func(w io.Writer) {}); err != nil {
		t.Fatalf("Emit(Progress) after switch back: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Graceful stop lifecycle
// ──────────────────────────────────────────────────────────────────────────────

func TestGracefulStop_Lifecycle(t *testing.T) {
	m := terminal.New(io.Discard)
	mustTransition(t, m, terminal.PhaseRunning)
	if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}
	// Ctrl+C arrives.
	mustTransition(t, m, terminal.PhaseStopping)
	// Final progress render.
	_ = m.Emit(terminal.OwnerProgress, func(w io.Writer) {})
	// Release progress.
	if err := m.ReleaseOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}
	// Advance to done.
	mustTransition(t, m, terminal.PhaseWaitingWorkers)
	mustTransition(t, m, terminal.PhaseFinalizing)
	mustTransition(t, m, terminal.PhaseDone)

	// Nothing can write now.
	assertPanics(t, "AcquireOwner after Done must panic", func() {
		_ = m.AcquireOwner(terminal.OwnerSummary)
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// AcquireAndTransition / TransitionAndRelease
// ──────────────────────────────────────────────────────────────────────────────

func TestAcquireAndTransition(t *testing.T) {
	m := terminal.New(io.Discard)
	if err := m.AcquireAndTransition(terminal.OwnerProgress, terminal.PhaseRunning); err != nil {
		t.Fatalf("AcquireAndTransition: %v", err)
	}
	if m.CurrentPhase() != terminal.PhaseRunning {
		t.Errorf("phase: want RUNNING, got %v", m.CurrentPhase())
	}
	if m.CurrentOwner() != terminal.OwnerProgress {
		t.Errorf("owner: want Progress, got %v", m.CurrentOwner())
	}
}

func TestTransitionAndRelease(t *testing.T) {
	m := terminal.New(io.Discard)
	mustTransition(t, m, terminal.PhaseRunning)
	if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}
	if err := m.TransitionAndRelease(terminal.PhaseWaitingWorkers, terminal.OwnerProgress); err != nil {
		t.Fatalf("TransitionAndRelease: %v", err)
	}
	if m.CurrentPhase() != terminal.PhaseWaitingWorkers {
		t.Errorf("phase: want WAITING_WORKERS, got %v", m.CurrentPhase())
	}
	if m.CurrentOwner() != terminal.OwnerNone {
		t.Errorf("owner: want None, got %v", m.CurrentOwner())
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Proof: progress + summary + pipeline cannot write simultaneously
// ──────────────────────────────────────────────────────────────────────────────

// TestConcurrentOwnersMathematicallyImpossible proves via the race detector
// that three goroutines claiming different owner roles cannot write simultaneously.
// Only the one that legitimately holds ownership can proceed; all others either
// wait on m.mu or panic (in test mode) when their Emit is rejected.
func TestConcurrentOwnersMathematicallyImpossible(t *testing.T) {
	var buf bytes.Buffer
	m := terminal.New(&buf)
	mustTransition(t, m, terminal.PhaseRunning)
	if err := m.AcquireOwner(terminal.OwnerProgress); err != nil {
		t.Fatal(err)
	}

	const N = 500
	var wg sync.WaitGroup
	var panicCount sync.WaitGroup

	// Progress goroutine — legitimately holds ownership.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < N; i++ {
			_ = m.Emit(terminal.OwnerProgress, func(w io.Writer) {
				fmt.Fprint(w, "P")
			})
		}
	}()

	// Summary goroutine — does NOT own; all Emits must panic/error.
	panicCount.Add(1)
	go func() {
		defer panicCount.Done()
		recovered := 0
		for i := 0; i < N; i++ {
			func() {
				defer func() {
					if r := recover(); r != nil {
						recovered++
					}
				}()
				_ = m.Emit(terminal.OwnerSummary, func(w io.Writer) {
					fmt.Fprint(w, "S") // must never execute
				})
			}()
		}
		if recovered != N {
			t.Errorf("Summary goroutine: expected %d panics, got %d", N, recovered)
		}
	}()

	// Pipeline goroutine — does NOT own; all Emits must panic/error.
	panicCount.Add(1)
	go func() {
		defer panicCount.Done()
		recovered := 0
		for i := 0; i < N; i++ {
			func() {
				defer func() {
					if r := recover(); r != nil {
						recovered++
					}
				}()
				_ = m.Emit(terminal.OwnerPipeline, func(w io.Writer) {
					fmt.Fprint(w, "X") // must never execute
				})
			}()
		}
		if recovered != N {
			t.Errorf("Pipeline goroutine: expected %d panics, got %d", N, recovered)
		}
	}()

	wg.Wait()
	panicCount.Wait()

	// Only 'P' must appear — no 'S' or 'X'.
	for i, c := range buf.String() {
		if c != 'P' {
			t.Fatalf("unexpected character %q at index %d (stdout corruption)", c, i)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

// assertPanics verifies that fn panics; fails the test if it does not.
func assertPanics(t *testing.T, label string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("%s: expected panic, got none", label)
		}
	}()
	fn()
}

// advanceTo creates a fresh Manager and advances it to the given phase.
// Panics on transition error (test setup failure).
func advanceTo(t *testing.T, target terminal.Phase) *terminal.Manager {
	t.Helper()
	m := terminal.New(io.Discard)
	sequence := []terminal.Phase{
		terminal.PhaseRunning,
		terminal.PhaseWaitingWorkers,
		terminal.PhaseFinalizing,
		terminal.PhaseTerminalShutdown,
		terminal.PhaseSummary,
		terminal.PhasePipeline,
		terminal.PhaseDone,
	}
	for _, p := range sequence {
		if err := m.TransitionTo(p); err != nil {
			t.Fatalf("advanceTo(%v): transition to %v failed: %v", target, p, err)
		}
		if p == target {
			return m
		}
	}
	t.Fatalf("advanceTo: phase %v not reachable in standard sequence", target)
	return nil
}

// mustTransition calls TransitionTo and fails the test on error.
func mustTransition(t *testing.T, m *terminal.Manager, to terminal.Phase) {
	t.Helper()
	if err := m.TransitionTo(to); err != nil {
		t.Fatalf("TransitionTo(%v): %v", to, err)
	}
}
