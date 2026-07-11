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
	"github.com/unsubble/searchit/internal/progress"
	"github.com/unsubble/searchit/internal/stats"
)

var stdoutMu sync.Mutex

// FakeRenderer implements progress.Renderer for testing.
type FakeRenderer struct {
	mu        sync.Mutex
	snapshots []stats.Snapshot
}

func (f *FakeRenderer) Render(snap stats.Snapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshots = append(f.snapshots, snap)
	return nil
}

func (f *FakeRenderer) Snapshots() []stats.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	copied := make([]stats.Snapshot, len(f.snapshots))
	copy(copied, f.snapshots)
	return copied
}

func TestManager_LifecycleAndCancellation(t *testing.T) {
	c := stats.NewCollector()
	c.RecordRequestSent()

	r := &FakeRenderer{}
	// Run with a very small refresh interval to verify it ticks fast enough for tests
	m := progress.NewManager(c, r, 5*time.Millisecond)

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

	snaps := r.Snapshots()
	if len(snaps) == 0 {
		t.Fatal("expected at least one render invocation, got 0")
	}

	finalSnap := snaps[len(snaps)-1]
	if finalSnap.RequestsSent != 1 {
		t.Errorf("expected final render snapshot to contain RequestsSent=1, got %d", finalSnap.RequestsSent)
	}
}

func TestTextRenderer_SnapshotRendering(t *testing.T) {
	c := stats.NewCollector()
	c.RecordRequestSent()
	c.RecordResponseReceived(200, 1024)
	c.SetActiveWorkers(10)
	c.SetQueuedJobs(50)
	c.RecordDiscovered()

	snap := c.Snapshot()

	var buf bytes.Buffer
	r := progress.NewTextRenderer(&buf, "https://target.local")
	err := r.Render(snap)
	if err != nil {
		t.Fatalf("unexpected rendering error: %v", err)
	}

	out := buf.String()
	expectedSubstrings := []string{
		"Target:      https://target.local",
		"Requests:    1",
		"Responses:   1",
		"Workers:     10",
		"Queue:       50",
		"Discovered:  1",
	}

	for _, s := range expectedSubstrings {
		if !bytes.Contains(buf.Bytes(), []byte(s)) {
			t.Errorf("expected output to contain %q, but got:\n%s", s, out)
		}
	}
}

func TestANSIRenderer_LifecycleAndCursor(t *testing.T) {
	var buf bytes.Buffer
	r := progress.NewANSIRenderer(&buf, "https://target.local", nil, "Single target")

	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("\033[?25l")) {
		t.Errorf("expected cursor to be hidden on creation, got: %q", out)
	}

	buf.Reset()
	err := r.Close()
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
	r := progress.NewANSIRenderer(&buf, "https://target.local", nil, "Single target")

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
	if !bytes.Contains(buf.Bytes(), []byte("200  /path7")) {
		t.Errorf("expected /path7 to be in output, but it was missing")
	}
}

func TestANSIRenderer_ANSIEscapeMovement(t *testing.T) {
	var buf bytes.Buffer
	r := progress.NewANSIRenderer(&buf, "https://target.local", nil, "Single target")

	c := stats.NewCollector()
	err := r.Render(c.Snapshot())
	if err != nil {
		t.Fatalf("unexpected rendering error: %v", err)
	}

	out1 := buf.String()
	if !strings.Contains(out1, "Target:") {
		t.Errorf("expected output to contain Target: header, got %q", out1)
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
	r := progress.NewTextRenderer(&buf, "http://localhost")
	m := progress.NewManager(c, r, 1*time.Second)

	m.PrintStats()

	out := buf.String()
	expectedSubstrings := []string{
		"--- Extended Statistics ---",
		"Elapsed:",
		"Requests:            1",
		"Responses:           2",
		"Discovered:          0",
		"Filtered:            1",
		"Failed:              1",
		"Bytes:               150",
		"Workers:             5",
		"Queue:               10",
		"Status Distribution",
		"200 : 1",
		"404 : 1",
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
	r := progress.NewANSIRenderer(f, "http://localhost", []string{"base", "php"}, "Recursive")
	defer r.Close()

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
	r := progress.NewANSIRenderer(&buf, "http://localhost", nil, "dfs")

	c := stats.NewCollector()
	snap := c.Snapshot()

	// Test case 1: <= 0 duration
	snap.AverageLatency = -5 * time.Second
	_ = r.Render(snap)
	if !strings.Contains(buf.String(), "Avg Latency:   -") {
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
	r := progress.NewANSIRenderer(&buf, "http://localhost/", nil, "dfs")

	// URL starts with target but does not have leading slash after trim
	r.AddResult(200, "http://localhost/path1")
	// URL does not start with target
	r.AddResult(301, "http://otherhost/path2")

	c := stats.NewCollector()
	_ = r.Render(c.Snapshot())

	out := buf.String()
	if !strings.Contains(out, "200  /path1") {
		t.Errorf("expected /path1 to be formatted, got:\n%s", out)
	}
	if !strings.Contains(out, "301  http://otherhost/path2") {
		t.Errorf("expected full URL path2 to be preserved, got:\n%s", out)
	}
}

func TestManager_CmdChan(t *testing.T) {
	c := stats.NewCollector()
	r := &FakeRenderer{}
	m := progress.NewManager(c, r, 100*time.Millisecond)

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
	r := progress.NewANSIRenderer(&buf, "http://localhost", nil, "bfs")
	m := progress.NewManager(c, r, 1*time.Second)

	// print stats triggers clear, print, and re-render
	m.PrintStats()

	out := buf.String()
	if !strings.Contains(out, "--- Extended Statistics ---") {
		t.Errorf("expected statistics output, got:\n%s", out)
	}
}

func TestManager_PrintStats_DefaultWriter(t *testing.T) {
	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	c := stats.NewCollector()
	r := &FakeRenderer{}
	m := progress.NewManager(c, r, 1*time.Second) // FakeRenderer is not Text/ANSI, writes to os.Stdout
	// Redirect os.Stdout to capture output
	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	m.PrintStats()
	pw.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, pr)
	out := buf.String()

	if !strings.Contains(out, "--- Extended Statistics ---") {
		t.Errorf("expected statistics on stdout, got:\n%s", out)
	}
}

func TestManager_RecordResult_NonANSI(t *testing.T) {
	c := stats.NewCollector()
	r := &FakeRenderer{}
	m := progress.NewManager(c, r, 1*time.Second)
	// Should not crash and do nothing
	m.RecordResult(200, "http://localhost/path")
}

func TestNewTextRenderer_NilWriter(t *testing.T) {
	r := progress.NewTextRenderer(nil, "http://localhost")
	if r == nil {
		t.Errorf("expected non-nil renderer")
	}
}

func TestNewANSIRenderer_NilWriter(t *testing.T) {
	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	oldStdout := os.Stdout
	_, pw, _ := os.Pipe()
	os.Stdout = pw

	r := progress.NewANSIRenderer(nil, "http://localhost", nil, "")
	_ = r.Close()

	pw.Close()
	os.Stdout = oldStdout
}

func TestNewManager_ZeroInterval(t *testing.T) {
	c := stats.NewCollector()
	r := &FakeRenderer{}
	m := progress.NewManager(c, r, 0)
	if m.Interval != 1*time.Second {
		t.Errorf("expected interval to fallback to 1s, got %v", m.Interval)
	}
}
