package stats_test

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/stats"
)

func TestCollector_Invariants_ProgressBounded(t *testing.T) {
	tests := []struct {
		name      string
		totalWork int64
		tried     int64
		skipped   int64
		check     func(t *testing.T, progress float64)
	}{
		{
			name:      "Zero Total Work",
			totalWork: 0,
			tried:     0,
			skipped:   0,
			check: func(t *testing.T, progress float64) {
				if progress != 0.0 {
					t.Errorf("expected 0.0, got %f", progress)
				}
			},
		},
		{
			name:      "Half Completed via Tried",
			totalWork: 200,
			tried:     100,
			skipped:   0,
			check: func(t *testing.T, progress float64) {
				if math.Abs(progress-50.0) > 0.001 {
					t.Errorf("expected 50.0%%, got %f", progress)
				}
			},
		},
		{
			name:      "Half Completed via Skipped and Tried",
			totalWork: 200,
			tried:     50,
			skipped:   50,
			check: func(t *testing.T, progress float64) {
				if math.Abs(progress-50.0) > 0.001 {
					t.Errorf("expected 50.0%%, got %f", progress)
				}
			},
		},
		{
			name:      "Fully Completed",
			totalWork: 500,
			tried:     250,
			skipped:   250,
			check: func(t *testing.T, progress float64) {
				if math.Abs(progress-100.0) > 0.001 {
					t.Errorf("expected 100.0%%, got %f", progress)
				}
			},
		},
		{
			name:      "Completed Exceeds Total (Dynamic Expansion)",
			totalWork: 100,
			tried:     150,
			skipped:   0,
			check: func(t *testing.T, progress float64) {
				if progress < 0 || math.IsNaN(progress) || math.IsInf(progress, 0) {
					t.Errorf("invalid progress: %f", progress)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := stats.NewCollector()
			c.SetTotalWork(tc.totalWork)
			for i := int64(0); i < tc.tried; i++ {
				c.RecordTried()
			}
			if tc.skipped > 0 {
				c.RecordSkipped(tc.skipped)
			}
			snap := c.Snapshot()
			tc.check(t, snap.Progress)
		})
	}
}

func TestCollector_Invariants_MonotonicityAndConservation(t *testing.T) {
	c := stats.NewCollector()

	// Simulate work pipeline
	totalCandidates := int64(100)
	c.SetTotalCandidates(totalCandidates)

	for i := int64(1); i <= totalCandidates; i++ {
		c.RecordJobProduced()
		c.RecordRequestSent()
		c.RecordTried()

		if i%2 == 0 {
			c.RecordResponseReceived(200, 1024)
			c.RecordRequestSucceeded()
			c.RecordDiscovered()
		} else if i%3 == 0 {
			c.RecordRequestFiltered()
			c.RecordResponseReceived(404, 0)
		} else {
			c.RecordRequestFailed()
		}

		snap := c.Snapshot()

		// Invariant: JobsProduced <= RequestsSent
		if snap.JobsProduced > snap.RequestsSent {
			t.Fatalf("Invariant violated at step %d: JobsProduced (%d) > RequestsSent (%d)", i, snap.JobsProduced, snap.RequestsSent)
		}
		// Invariant: Discovered <= RequestsSent
		if snap.Discovered > snap.RequestsSent {
			t.Fatalf("Invariant violated at step %d: Discovered (%d) > RequestsSent (%d)", i, snap.Discovered, snap.RequestsSent)
		}
		// Invariant: Discovered <= ResponsesReceived
		if snap.Discovered > snap.ResponsesReceived {
			t.Fatalf("Invariant violated at step %d: Discovered (%d) > ResponsesReceived (%d)", i, snap.Discovered, snap.ResponsesReceived)
		}
		// Invariant: SearchSpaceProgress == Completed
		if snap.SearchSpaceProgress != snap.Completed {
			t.Fatalf("SearchSpaceProgress (%d) != Completed (%d)", snap.SearchSpaceProgress, snap.Completed)
		}
		// Invariant: TotalCandidates == TotalWork
		if snap.TotalCandidates != snap.TotalWork {
			t.Fatalf("TotalCandidates (%d) != TotalWork (%d)", snap.TotalCandidates, snap.TotalWork)
		}
		// Invariant: Completed == Tried + Skipped
		if snap.Completed != (snap.Tried + snap.Skipped) {
			t.Fatalf("Completed (%d) != Tried (%d) + Skipped (%d)", snap.Completed, snap.Tried, snap.Skipped)
		}
	}
}

func TestCollector_Invariants_RatesNonNegativeAndFinite(t *testing.T) {
	c := stats.NewCollector()

	// Snapshot at instant 0
	snap := c.Snapshot()
	if math.IsNaN(snap.RequestsPerSecond) || math.IsInf(snap.RequestsPerSecond, 0) || snap.RequestsPerSecond < 0 {
		t.Errorf("invalid RequestsPerSecond: %f", snap.RequestsPerSecond)
	}
	if math.IsNaN(snap.CurrentRequestsPerSecond) || math.IsInf(snap.CurrentRequestsPerSecond, 0) || snap.CurrentRequestsPerSecond < 0 {
		t.Errorf("invalid CurrentRequestsPerSecond: %f", snap.CurrentRequestsPerSecond)
	}

	// Add work and check again
	c.RecordRequestSent()
	c.RecordResponseReceived(200, 500)
	time.Sleep(10 * time.Millisecond)

	snap2 := c.Snapshot()
	if snap2.RequestsPerSecond < 0 || math.IsNaN(snap2.RequestsPerSecond) {
		t.Errorf("invalid RequestsPerSecond: %f", snap2.RequestsPerSecond)
	}
}

func TestCollector_Invariants_ConcurrentSafety(t *testing.T) {
	c := stats.NewCollector()
	c.SetTotalCandidates(10000)

	var wg sync.WaitGroup
	workers := 10
	iterations := 100

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				c.RecordJobProduced()
				c.RecordRequestSent()
				c.RecordResponseReceived(200, 100)
				c.RecordDiscovered()
				c.RecordRetry()
				c.RecordRedirect()
				c.IncrementActiveWorkers()
				c.DecrementActiveWorkers()
				_ = c.Snapshot()
			}
		}()
	}

	wg.Wait()

	finalSnap := c.Snapshot()
	expectedWork := int64(workers * iterations)
	if finalSnap.JobsProduced != expectedWork {
		t.Errorf("expected %d jobs produced, got %d", expectedWork, finalSnap.JobsProduced)
	}
	if finalSnap.RequestsSent != expectedWork {
		t.Errorf("expected %d requests sent, got %d", expectedWork, finalSnap.RequestsSent)
	}
	if finalSnap.Discovered != expectedWork {
		t.Errorf("expected %d discoveries, got %d", expectedWork, finalSnap.Discovered)
	}
	if finalSnap.ActiveWorkers != 0 {
		t.Errorf("expected 0 active workers at end, got %d", finalSnap.ActiveWorkers)
	}
}
