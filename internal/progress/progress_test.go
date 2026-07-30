package progress_test

import (
	"bytes"
	"context"
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
		"Statistics",
		"General",
		"http://localhost",
		"bfs",
		"Requests sent",
		"PERFORMANCE",
		"Total requests",
		"TIMING",
		"Elapsed",
		"Workers",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(out, sub) {
			t.Errorf("expected output to contain %q, but got:\n%s", sub, out)
		}
	}
}

func TestANSIRenderer_TerminalAndFrozen(t *testing.T) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		t.Skip("skipping /dev/tty test: controlling terminal not available")
		return
	}
	defer f.Close()

	// instantiate ANSIRenderer with real terminal file
	tm := terminal.New(f)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", []string{"base", "php"}, "Recursive")
	defer r.Close(terminal.OwnerProgress)

	c := stats.NewCollector()
	snap := c.Snapshot()
	snap.ActiveWorkers = 4
	snap.TotalCandidates = 10
	snap.RequestsPerSecond = 5.0
	snap.CurrentRequestsPerSecond = 5.0

	err = r.Render(snap)
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
		if !strings.Contains(out, "Directories: Discovered: 184 │ Queued: 12") {
			t.Errorf("expected exact directory activity metrics in output, got:\n%s", out)
		}
	})
}
