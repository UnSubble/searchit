package progress_test

import (
	"bytes"
	"context"
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
		m.Start(ctx)
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
