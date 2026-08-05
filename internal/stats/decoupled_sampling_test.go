package stats_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/stats"
)

// TestSnapshot_NoStateMutation verifies that Snapshot() performs no state mutation
// and calling Snapshot() 1000 times without new requests returns identical throughput.
func TestSnapshot_NoStateMutation(t *testing.T) {
	c := stats.NewCollector()

	// Simulate some requests
	for i := 0; i < 500; i++ {
		c.RecordRequestSent()
	}
	c.Sample()

	firstSnap := c.Snapshot()

	// Call Snapshot() 1000 times without new requests
	for i := 0; i < 1000; i++ {
		snap := c.Snapshot()
		if snap.CurrentRequestsPerSecond != firstSnap.CurrentRequestsPerSecond {
			t.Fatalf("iteration %d: CurrentRequestsPerSecond changed from %f to %f without new requests",
				i, firstSnap.CurrentRequestsPerSecond, snap.CurrentRequestsPerSecond)
		}
		if snap.RequestsSent != firstSnap.RequestsSent {
			t.Fatalf("iteration %d: RequestsSent changed from %d to %d without new requests",
				i, firstSnap.RequestsSent, snap.RequestsSent)
		}
	}
}

// TestSnapshot_IdenticalSnapshots verifies that repeated calls to Snapshot()
// without new requests produce identical Snapshot struct results.
func TestSnapshot_IdenticalSnapshots(t *testing.T) {
	c := stats.NewCollector()

	for i := 0; i < 100; i++ {
		c.RecordRequestSent()
	}

	snap1 := c.Snapshot()
	snap2 := c.Snapshot()

	if !reflect.DeepEqual(snap1.StatusCodes, snap2.StatusCodes) {
		t.Fatalf("StatusCodes differ between consecutive Snapshot calls")
	}
	if snap1.CurrentRequestsPerSecond != snap2.CurrentRequestsPerSecond {
		t.Fatalf("CurrentRequestsPerSecond = %f, want %f", snap2.CurrentRequestsPerSecond, snap1.CurrentRequestsPerSecond)
	}
	if snap1.RequestsSent != snap2.RequestsSent {
		t.Fatalf("RequestsSent = %d, want %d", snap2.RequestsSent, snap1.RequestsSent)
	}
}

// TestSampling_FrequentSnapshotCallsDoesNotAlterReqPerSec verifies that frequent
// Snapshot() / redraw calls do not alter or corrupt CurrentRequestsPerSecond.
func TestSampling_FrequentSnapshotCallsDoesNotAlterReqPerSec(t *testing.T) {
	c := stats.NewCollector()

	// Continuous request generator at ~1000 req/s
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				c.RecordRequestSent()
			}
		}
	}()

	time.Sleep(500 * time.Millisecond)

	baselineSnap := c.Snapshot()

	defer close(done)

	// Simulate 500 high-frequency UI redraw Snapshot() calls
	for i := 0; i < 500; i++ {
		c.Snapshot()
	}

	finalSnap := c.Snapshot()

	// Rate must remain non-zero and stable (no collapse to 0)
	if finalSnap.CurrentRequestsPerSecond <= 0 {
		t.Fatalf("CurrentRequestsPerSecond collapsed to %f under high-frequency Snapshot calls",
			finalSnap.CurrentRequestsPerSecond)
	}

	t.Logf("Baseline Req/s: %f, Final Req/s: %f", baselineSnap.CurrentRequestsPerSecond, finalSnap.CurrentRequestsPerSecond)
}

// TestSampling_NoCollapseOnRingBufferRollover verifies that CurrentRequestsPerSecond
// never collapses to 0 while requests are continuously being dispatched over multiple 5s cycles.
func TestSampling_NoCollapseOnRingBufferRollover(t *testing.T) {
	c := stats.NewCollector()

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Microsecond) // ~2000 req/s
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				c.RecordRequestSent()
			}
		}
	}()

	time.Sleep(500 * time.Millisecond)

	defer close(done)

	// Monitor throughput for 6 seconds (exceeding 5s ring-buffer rollover period)
	start := time.Now()
	for time.Since(start) < 6*time.Second {
		time.Sleep(50 * time.Millisecond)
		snap := c.Snapshot()
		if snap.CurrentRequestsPerSecond <= 0 {
			t.Fatalf("CurrentRequestsPerSecond collapsed to %f at elapsed time %v (ring-buffer rollover bug)",
				snap.CurrentRequestsPerSecond, time.Since(start))
		}
	}
}
