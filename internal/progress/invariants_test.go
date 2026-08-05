package progress_test

import (
	"bytes"
	"context"
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/unsubble/searchit/internal/console"
	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/progress"
	"github.com/unsubble/searchit/internal/stats"
)

func TestProgressBar_Invariants(t *testing.T) {
	tests := []struct {
		name     string
		progress float64
		width    int
	}{
		{"Zero Progress", 0.0, 20},
		{"Quarter Progress", 25.0, 20},
		{"Half Progress", 50.0, 20},
		{"Full Progress", 100.0, 20},
		{"Negative Progress Clamping", -50.0, 20},
		{"Overflow Progress Clamping", 150.0, 20},
		{"NaN Progress Clamping", math.NaN(), 20},
		{"Positive Inf Progress Clamping", math.Inf(1), 20},
		{"Negative Inf Progress Clamping", math.Inf(-1), 20},
		{"Custom Width 10", 33.3, 10},
		{"Custom Width 40", 80.0, 40},
		{"Default Fallback Width for 0", 50.0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bar := progress.ProgressBar(tc.progress, tc.width)
			expectedWidth := tc.width
			if expectedWidth <= 0 {
				expectedWidth = progress.ProgressBarWidth
			}
			expectedRuneLen := expectedWidth + 2 // '[' + bar + ']'

			actualRuneLen := utf8.RuneCountInString(bar)
			if actualRuneLen != expectedRuneLen {
				t.Errorf("expected visual rune length %d, got %d for bar %q", expectedRuneLen, actualRuneLen, bar)
			}
			if !strings.HasPrefix(bar, "[") || !strings.HasSuffix(bar, "]") {
				t.Errorf("expected bar to be enclosed in brackets, got %s", bar)
			}
		})
	}
}

func TestANSIRenderer_FiniteAndOpenEnded_Invariants(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireAndTransition(terminal.OwnerProgress, terminal.PhaseRunning)

	renderer := progress.NewANSIRenderer(tm, "http://target.local", []string{"default"}, "standard")

	// 1. Finite snapshot
	finiteSnap := stats.Snapshot{
		StartTime:         time.Now().Add(-5 * time.Second),
		IsFinite:          true,
		TotalWork:         1000,
		Tried:             400,
		Skipped:           100,
		RequestsSent:      400,
		ResponsesReceived: 390,
		RequestsFailed:    10,
		Discovered:        15,
		ActiveWorkers:     8,
	}

	err := renderer.Render(finiteSnap)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Progress:") || !strings.Contains(out, "500 / 1000 (50.0%)") {
		t.Errorf("expected finite progress bar in output, got:\n%s", out)
	}

	// 2. Open-ended / Recursive snapshot
	buf.Reset()
	openEndedSnap := stats.Snapshot{
		StartTime:             time.Now().Add(-5 * time.Second),
		IsFinite:              false,
		RequestsSent:          500,
		DirectoriesDiscovered: 12,
		FrontierPending:       4,
		ActiveWorkers:         4,
	}

	err = renderer.Render(openEndedSnap)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	outOpen := buf.String()
	if !strings.Contains(outOpen, "Recursion: Expanded: 12") || !strings.Contains(outOpen, "Pending: 4") {
		t.Errorf("expected recursive metrics in output, got:\n%s", outOpen)
	}
}

func TestManager_LifecycleAndCommands(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireAndTransition(terminal.OwnerProgress, terminal.PhaseRunning)

	c := stats.NewCollector()
	c.SetTotalWork(100)
	c.RecordJobProduced()
	c.RecordRequestSent()

	renderer := progress.NewANSIRenderer(tm, "http://target.local", nil, "scan")
	mgr := progress.NewManager(tm, c, renderer, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cmdChan := make(chan console.Command, 5)

	done := make(chan struct{})
	go func() {
		mgr.Start(ctx, cmdChan)
		close(done)
	}()

	time.Sleep(25 * time.Millisecond)

	// Send progress command
	cmdChan <- console.CommandProgress
	time.Sleep(15 * time.Millisecond)

	// Send stats command
	cmdChan <- console.CommandStats
	time.Sleep(15 * time.Millisecond)

	// Send progress command again to toggle back
	cmdChan <- console.CommandProgress
	time.Sleep(15 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("Manager did not shut down within timeout")
	}
}
