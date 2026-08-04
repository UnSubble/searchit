package progress_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/console"
	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/progress"
	"github.com/unsubble/searchit/internal/stats"
)

var stdoutMu sync.Mutex

func TestManager_LifecycleAndCancellation(t *testing.T) {
	c := stats.NewCollector()
	c.RecordRequestSent()

	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "bfs")
	// Run with a very small refresh interval to verify it ticks fast enough for tests
	m := progress.NewManager(tm, c, r, 5*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		m.Start(ctx, nil)
		close(done)
	}()

	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("manager did not exit cleanly upon context cancellation")
	}

	if buf.Len() == 0 {
		t.Fatal("expected at least one render invocation, got 0")
	}
}

func TestANSIRenderer_LifecycleAndCursor(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "https://target.local", nil, "Single target")

	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("\033[?25l")) {
		t.Errorf("expected cursor to be hidden on creation, got: %q", out)
	}

	buf.Reset()
	err := r.Close(terminal.OwnerProgress)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out = buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("\033[?25h")) {
		t.Errorf("expected cursor to be shown on Close, got: %q", out)
	}
}

func TestANSIRenderer_ANSIEscapeMovement(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "https://target.local", nil, "Single target")

	c := stats.NewCollector()
	c.SetIsFinite(true)
	err := r.Render(c.Snapshot())
	if err != nil {
		t.Fatalf("unexpected rendering error: %v", err)
	}

	if !strings.Contains(buf.String(), "Progress") {
		t.Errorf("expected output to contain Progress header, got %q", buf.String())
	}
}

func TestManager_PrintStats(t *testing.T) {
	c := stats.NewCollector()
	c.RecordRequestSent()
	c.RecordResponseReceived(200, 100)
	c.RecordResponseReceived(404, 50)
	c.RecordRequestFiltered()
	c.RecordRequestFailed()
	c.SetActiveWorkers(5)
	c.SetTotalCandidates(10)

	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "bfs")
	m := progress.NewManager(tm, c, r, 1*time.Second)

	m.PrintStats()

	out := buf.String()
	expectedSubstrings := []string{
		"Statistics (press any key to return)",
		"Method",
		"HTTP",
		"Requests Sent",
		"Findings",
		"Errors",
		"Retries",
		"Bytes Received",
		"Total Candidates",
		"Total Req/sec",
		"Elapsed",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(out, sub) {
			t.Errorf("expected output to contain %q, but got:\n%s", sub, out)
		}
	}
}

func TestANSIRenderer_TerminalAndFrozen(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", []string{"base", "php"}, "Recursive")
	defer r.Close(terminal.OwnerProgress)

	c := stats.NewCollector()
	snap := c.Snapshot()
	snap.ActiveWorkers = 4
	snap.TotalCandidates = 10
	snap.RequestsPerSecond = 5.0
	snap.CurrentRequestsPerSecond = 5.0

	err := r.Render(snap)
	if err != nil {
		t.Fatalf("unexpected Render error: %v", err)
	}

	r.Clear()
}

func TestManager_CmdChan(t *testing.T) {
	c := stats.NewCollector()
	tm := terminal.New(io.Discard)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "bfs")
	m := progress.NewManager(tm, c, r, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmdChan := make(chan console.Command)
	done := make(chan struct{})
	go func() {
		m.Start(ctx, cmdChan)
		close(done)
	}()

	// Send CommandProgress
	cmdChan <- console.CommandProgress
	// Send CommandStats
	cmdChan <- console.CommandStats

	time.Sleep(15 * time.Millisecond)

	// Close command channel
	close(cmdChan)
	time.Sleep(15 * time.Millisecond)

	cancel()
	<-done
}

func TestManager_PrintStats_ANSIRenderer(t *testing.T) {
	c := stats.NewCollector()
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "bfs")
	m := progress.NewManager(tm, c, r, 1*time.Second)

	// print stats triggers clear, print, and re-render
	m.PrintStats()

	out := buf.String()
	if !strings.Contains(out, "Statistics") {
		t.Errorf("expected statistics output, got:\n%s", out)
	}
}

func TestManager_PrintStats_DefaultWriter(t *testing.T) {
	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	// Redirect os.Stdout to capture output
	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	c := stats.NewCollector()
	tm := terminal.New(pw)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "bfs")
	m := progress.NewManager(tm, c, r, 1*time.Second)

	m.PrintStats()
	pw.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, pr)
	out := buf.String()

	if !strings.Contains(out, "Statistics") {
		t.Errorf("expected statistics on stdout, got:\n%s", out)
	}
}

func TestNewANSIRenderer_NilWriter(t *testing.T) {
	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	oldStdout := os.Stdout
	_, pw, _ := os.Pipe()
	os.Stdout = pw

	tm := terminal.New(nil)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "")
	_ = r.Close(terminal.OwnerProgress)

	pw.Close()
	os.Stdout = oldStdout
}

func TestNewManager_ZeroInterval(t *testing.T) {
	c := stats.NewCollector()
	tm := terminal.New(io.Discard)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "bfs")
	m := progress.NewManager(tm, c, r, 0)
	if m.Interval != 1*time.Second {
		t.Errorf("expected interval to fallback to 1s, got %v", m.Interval)
	}
}

func TestProgress_StatsViewAndNumberFormatting(t *testing.T) {
	var buf bytes.Buffer
	c := stats.NewCollector()
	c.RecordResponseReceived(200, 1234567) // 1.2M bytes

	snap := c.Snapshot()
	snap.RequestsSent = 1500000 // 1.5M

	progress.RenderStatsView(&buf, snap, 32)
	out := buf.String()

	if !strings.Contains(out, "1.5M") {
		t.Errorf("expected formatted number 1.5M in output, got:\n%s", out)
	}
}

func TestANSIRenderer_ResetLineCount(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "bfs")
	r.ResetLineCount()
}

func TestANSIRenderer_FiniteVsOpenEndedProgress(t *testing.T) {
	t.Run("Finite Scan Output", func(t *testing.T) {
		var buf bytes.Buffer
		tm := terminal.New(&buf)
		_ = tm.AcquireOwner(terminal.OwnerProgress)
		r := progress.NewANSIRenderer(tm, "http://localhost", nil, "Single target")

		c := stats.NewCollector()
		c.SetIsFinite(true)
		c.SetTotalWork(4000)
		c.RecordTried()
		c.RecordTried()
		c.RecordResponseReceived(200, 100)

		snap := c.Snapshot()
		err := r.Render(snap)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "Progress:") || !strings.Contains(out, "/ 4000") {
			t.Errorf("expected finite progress output with exact ratio, got:\n%s", out)
		}
	})

	t.Run("Open-Ended Recursive Scan Output", func(t *testing.T) {
		var buf bytes.Buffer
		tm := terminal.New(&buf)
		_ = tm.AcquireOwner(terminal.OwnerProgress)
		r := progress.NewANSIRenderer(tm, "http://localhost", nil, "Recursive (BFS)")

		c := stats.NewCollector()
		c.SetIsFinite(false)
		c.RecordDirectoryDiscovered()
		c.SetFrontierPending(12)
		c.SetDirectories(184, 12)

		snap := c.Snapshot()
		err := r.Render(snap)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := buf.String()
		if strings.Contains(out, "Progress:") || strings.Contains(out, "ETA") {
			t.Errorf("expected NO progress bar or ETA in open-ended scan output, got:\n%s", out)
		}
		if !strings.Contains(out, "Requests Sent:") {
			t.Errorf("expected 'Requests Sent:' header in open-ended scan output, got:\n%s", out)
		}
		if !strings.Contains(out, "Recursion: Expanded: 184 │ Pending: 12") {
			t.Errorf("expected exact recursion activity metrics in output, got:\n%s", out)
		}
	})
}

type safeWriter struct {
	buf *bytes.Buffer
	mu  *sync.Mutex
}

func (w *safeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func TestManager_InPlaceStatisticsOverlay(t *testing.T) {
	c := stats.NewCollector()
	var buf bytes.Buffer
	var bufMu sync.Mutex
	writer := &safeWriter{buf: &buf, mu: &bufMu}
	tm := terminal.New(writer)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "bfs")
	m := progress.NewManager(tm, c, r, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmdChan := make(chan console.Command, 10)
	done := make(chan struct{})
	go func() {
		m.Start(ctx, cmdChan)
		close(done)
	}()

	// 1. Enter Stats view by sending CommandStats
	cmdChan <- console.CommandStats
	time.Sleep(30 * time.Millisecond)

	bufMu.Lock()
	out := buf.String()
	bufMu.Unlock()
	if !strings.Contains(out, "Statistics") {
		t.Errorf("expected statistics report header in output, got:\n%s", out)
	}

	// 2. Execute a finding above while stats view is open
	var findingExecuted bool
	m.ExecuteAbove(func() {
		findingExecuted = true
	})

	if !findingExecuted {
		t.Error("expected finding to be executed immediately above the in-place stats overlay")
	}

	// 3. Exit Stats view by sending CommandStats again (toggle)
	cmdChan <- console.CommandStats
	time.Sleep(30 * time.Millisecond)

	bufMu.Lock()
	out = buf.String()
	bufMu.Unlock()
	if !strings.Contains(out, "Requests Sent:") && !strings.Contains(out, "Progress:") {
		t.Errorf("expected normal progress panel restored after exiting stats view, got:\n%s", out)
	}

	cancel()
	<-done
}

func TestManager_StatsViewCleanupOnShutdown(t *testing.T) {
	c := stats.NewCollector()
	var buf bytes.Buffer
	var bufMu sync.Mutex
	writer := &safeWriter{buf: &buf, mu: &bufMu}
	tm := terminal.New(writer)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "fuzz")
	m := progress.NewManager(tm, c, r, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cmdChan := make(chan console.Command, 10)
	done := make(chan struct{})
	go func() {
		m.Start(ctx, cmdChan)
		close(done)
	}()

	// Enter Stats view
	cmdChan <- console.CommandStats
	time.Sleep(30 * time.Millisecond)

	// Cancel context directly while stats view is active
	cancel()
	<-done

	// Call Close to finalize renderer teardown
	_ = r.Close(terminal.OwnerProgress)

	bufMu.Lock()
	out := buf.String()
	bufMu.Unlock()

	if !strings.Contains(out, "\033[J") {
		t.Errorf("expected ANSI clear sequence \\033[J on shutdown from stats view, got:\n%s", out)
	}
}

func TestProgressBar_ConstantWidth(t *testing.T) {
	testCases := []struct {
		name       string
		percentage float64
		width      int
	}{
		{"zero percent", 0.0, progress.ProgressBarWidth},
		{"ten percent", 10.0, progress.ProgressBarWidth},
		{"twenty five percent", 25.0, progress.ProgressBarWidth},
		{"fifty percent", 50.0, progress.ProgressBarWidth},
		{"seventy five percent", 75.0, progress.ProgressBarWidth},
		{"one hundred percent", 100.0, progress.ProgressBarWidth},
		{"negative clamped", -15.0, progress.ProgressBarWidth},
		{"over hundred clamped", 125.0, progress.ProgressBarWidth},
		{"custom width 10", 50.0, 10},
		{"custom width 30", 50.0, 30},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bar := progress.ProgressBar(tc.percentage, tc.width)
			expectedRuneCount := tc.width + 2 // '[' + width characters + ']'
			runes := []rune(bar)
			if len(runes) != expectedRuneCount {
				t.Fatalf("expected bar %q to have rune count %d, got %d", bar, expectedRuneCount, len(runes))
			}
			if !strings.HasPrefix(bar, "[") || !strings.HasSuffix(bar, "]") {
				t.Fatalf("expected bar %q to be enclosed in brackets", bar)
			}
		})
	}
}

func TestProgressBar_VariousCounters_ConstantWidth(t *testing.T) {
	cases := []struct {
		completed int64
		total     int64
	}{
		{completed: 1, total: 10},
		{completed: 10, total: 100},
		{completed: 100, total: 1000},
		{completed: 10000, total: 500000},
	}

	var extractedBars []string
	for _, tc := range cases {
		c := stats.NewCollector()
		c.SetIsFinite(true)
		c.SetTotalWork(tc.total)
		for i := int64(0); i < tc.completed; i++ {
			c.RecordTried()
		}

		var buf bytes.Buffer
		tm := terminal.New(&buf)
		_ = tm.AcquireOwner(terminal.OwnerProgress)
		r := progress.NewANSIRenderer(tm, "http://localhost", nil, "Standard")

		err := r.Render(c.Snapshot())
		if err != nil {
			t.Fatalf("unexpected render error: %v", err)
		}

		out := buf.String()
		// Find the progress bar within the rendered output: between '[' and ']'
		start := strings.Index(out, "Progress: [")
		if start == -1 {
			t.Fatalf("could not find 'Progress: [' in output:\n%s", out)
		}
		barStart := start + len("Progress: ")
		barEnd := strings.Index(out[barStart:], "]")
		if barEnd == -1 {
			t.Fatalf("could not find closing ']' in output:\n%s", out)
		}
		bar := out[barStart : barStart+barEnd+1]
		extractedBars = append(extractedBars, bar)
	}

	firstBarRunes := len([]rune(extractedBars[0]))
	if firstBarRunes != progress.ProgressBarWidth+2 {
		t.Fatalf("expected initial bar width to be %d, got %d for bar %q", progress.ProgressBarWidth+2, firstBarRunes, extractedBars[0])
	}

	for i, bar := range extractedBars {
		runeCount := len([]rune(bar))
		if runeCount != firstBarRunes {
			t.Errorf("case %d (%d/%d): expected bar length %d, got %d (%q)",
				i, cases[i].completed, cases[i].total, firstBarRunes, runeCount, bar)
		}
	}
}

// TestProgressBar_ConstantDenominatorThroughoutScan verifies that during execution,
// as jobs are produced and tried, the progress line denominator remains strictly constant (e.g. X / 57005).
func TestProgressBar_ConstantDenominatorThroughoutScan(t *testing.T) {
	const total = int64(57005)
	c := stats.NewCollector()
	c.SetIsFinite(true)
	c.SetTotalWork(total)

	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "Standard")

	checkpoints := []int64{1, 50, 100, 504, 1364, 2410, 12453, 57005}
	lastTried := int64(0)

	for _, cp := range checkpoints {
		for i := lastTried; i < cp; i++ {
			c.RecordJobProduced()
			c.AddTotalCandidates(1) // Legacy call simulation — must be ignored and not mutate total
			c.RecordTried()
		}
		lastTried = cp

		buf.Reset()
		err := r.Render(c.Snapshot())
		if err != nil {
			t.Fatalf("render error at %d: %v", cp, err)
		}

		out := buf.String()
		expectedPattern := fmt.Sprintf("%d / %d", cp, total)
		if !strings.Contains(out, expectedPattern) {
			t.Errorf("rendered progress at completed=%d does not contain expected pattern %q. Rendered output:\n%s", cp, expectedPattern, out)
		}
	}
}
