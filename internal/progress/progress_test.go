package progress_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/progress"
	"github.com/unsubble/searchit/internal/stats"
)

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
