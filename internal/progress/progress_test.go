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
	"github.com/unsubble/searchit/internal/engine"
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

func TestANSIRenderer_RecentResultBuffer(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "https://target.local", nil, "Single target")

	for i := 1; i <= 7; i++ {
		r.AddResult(200, fmt.Sprintf("https://target.local/path%d", i))
	}

	c := stats.NewCollector()
	err := r.Render(c.Snapshot())
	if err != nil {
		t.Fatalf("unexpected rendering error: %v", err)
	}

	if bytes.Contains(buf.Bytes(), []byte("  /path1\n")) {
		t.Errorf("expected /path1 to be evicted, but it was found in output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("200 /path7")) {
		t.Errorf("expected /path7 to be in output, but it was missing")
	}
}

func TestANSIRenderer_ANSIEscapeMovement(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "https://target.local", nil, "Single target")

	c := stats.NewCollector()
	err := r.Render(c.Snapshot())
	if err != nil {
		t.Fatalf("unexpected rendering error: %v", err)
	}

	if !strings.Contains(buf.String(), "Target") {
		t.Errorf("expected output to contain Target header, got %q", buf.String())
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
	c.SetQueuedJobs(10)

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
		"Requests sent",
		"Responses received",
		"Filtered",
		"Failed",
		"Bytes received",
		"Performance",
		"Elapsed",
		"Workers",
		"Active",
		"Queue",
		"Responses",
		"200",
		"404",
		"Press any key to return",
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
	snap.QueuedJobs = 20
	snap.RequestsPerSecond = 5.0

	err = r.Render(snap)
	if err != nil {
		t.Fatalf("unexpected Render error: %v", err)
	}

	r.Clear()
}

func TestANSIRenderer_FormatLatency_All(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "dfs")

	c := stats.NewCollector()
	snap := c.Snapshot()

	snap.AverageLatency = -5 * time.Second
	_ = r.Render(snap)
	if !strings.Contains(buf.String(), "Latency     -") {
		t.Errorf("expected '-' for negative latency, got: %s", buf.String())
	}
	buf.Reset()

	// Test case 2: < 1ms duration (microseconds)
	snap.AverageLatency = 500 * time.Microsecond
	_ = r.Render(snap)
	if !strings.Contains(buf.String(), "500µs") {
		t.Errorf("expected '500µs', got: %s", buf.String())
	}
	buf.Reset()

	// Test case 3: < 1s duration (milliseconds)
	snap.AverageLatency = 250 * time.Millisecond
	_ = r.Render(snap)
	if !strings.Contains(buf.String(), "250ms") {
		t.Errorf("expected '250ms', got: %s", buf.String())
	}
	buf.Reset()

	// Test case 4: >= 1s duration (seconds)
	snap.AverageLatency = 3500 * time.Millisecond
	_ = r.Render(snap)
	if !strings.Contains(buf.String(), "3.50s") {
		t.Errorf("expected '3.50s', got: %s", buf.String())
	}
}

func TestANSIRenderer_ExtractPath_All(t *testing.T) {
	var buf bytes.Buffer
	// target ends with trailing slash
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost/", nil, "dfs")

	// URL starts with target but does not have leading slash after trim
	r.AddResult(200, "http://localhost/path1")
	// URL does not start with target
	r.AddResult(301, "http://otherhost/path2")

	c := stats.NewCollector()
	_ = r.Render(c.Snapshot())

	out := buf.String()
	if !strings.Contains(out, "200 /path1") {
		t.Errorf("expected /path1 to be formatted, got:\n%s", out)
	}
	if !strings.Contains(out, "301 http://otherhost/path2") {
		t.Errorf("expected full URL path2 to be preserved, got:\n%s", out)
	}
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

func TestManager_RecordResult_ANSI(t *testing.T) {
	c := stats.NewCollector()
	tm := terminal.New(io.Discard)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "bfs")
	m := progress.NewManager(tm, c, r, 1*time.Second)
	// Should not crash and do nothing
	m.RecordResult(200, "http://localhost/path")
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

	progress.RenderStatsView(&buf, snap, 32, nil)
	out := buf.String()

	if !strings.Contains(out, "1,500,000") {
		t.Errorf("expected formatted number 1,500,000 in output, got:\n%s", out)
	}
}

func TestProgress_HandleResult(t *testing.T) {
	c := stats.NewCollector()
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "bfs")
	m := progress.NewManager(tm, c, r, 1*time.Second)

	res := engine.Result{
		URL:        "http://localhost/admin",
		StatusCode: 200,
		Length:     42,
		Depth:      1,
	}

	m.HandleResult(res)

	// Since live dashboard is active, result must be routed and output
	out := buf.String()
	if !strings.Contains(out, "200") || !strings.Contains(out, "/admin") {
		t.Errorf("expected result to be printed, got:\n%s", out)
	}
}

func TestANSIRenderer_ResetLineCount(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerProgress)
	r := progress.NewANSIRenderer(tm, "http://localhost", nil, "bfs")
	r.ResetLineCount()
}
