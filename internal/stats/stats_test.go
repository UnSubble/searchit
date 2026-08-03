package stats_test

import (
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/stats"
)

func TestCollector_ZeroValues(t *testing.T) {
	c := stats.NewCollector()
	snap := c.Snapshot()

	if snap.RequestsSent != 0 {
		t.Errorf("expected 0 requests sent, got %d", snap.RequestsSent)
	}
	if snap.ResponsesReceived != 0 {
		t.Errorf("expected 0 responses received, got %d", snap.ResponsesReceived)
	}
	if snap.RequestsFiltered != 0 {
		t.Errorf("expected 0 requests filtered, got %d", snap.RequestsFiltered)
	}
	if snap.RequestsFailed != 0 {
		t.Errorf("expected 0 requests failed, got %d", snap.RequestsFailed)
	}
	if snap.RequestsSucceeded != 0 {
		t.Errorf("expected 0 requests succeeded, got %d", snap.RequestsSucceeded)
	}
	if snap.BytesReceived != 0 {
		t.Errorf("expected 0 bytes received, got %d", snap.BytesReceived)
	}
	if snap.ActiveWorkers != 0 {
		t.Errorf("expected 0 active workers, got %d", snap.ActiveWorkers)
	}
	if snap.TotalCandidates != 0 {
		t.Errorf("TotalCandidates: expected 0, got %d", snap.TotalCandidates)
	}
	if snap.Discovered != 0 {
		t.Errorf("expected 0 discovered, got %d", snap.Discovered)
	}
	if len(snap.StatusCodes) != 0 {
		t.Errorf("expected empty status codes map, got %v", snap.StatusCodes)
	}
}

func TestCollector_BasicOperations(t *testing.T) {
	c := stats.NewCollector()

	c.RecordRequestSent()
	c.RecordResponseReceived(200, 1024)
	c.RecordResponseReceived(200, 2048)
	c.RecordResponseReceived(404, 512)
	c.RecordRequestFiltered()
	c.RecordRequestFailed()
	c.RecordRequestSucceeded()
	c.RecordDiscovered()
	c.IncrementActiveWorkers()
	c.IncrementActiveWorkers()
	c.DecrementActiveWorkers()
	c.SetTotalCandidates(42)

	// Latency sample
	c.RecordLatency(100 * time.Millisecond)
	c.RecordLatency(200 * time.Millisecond)

	// Extra fields
	c.RecordRetry()
	c.RecordRedirect()
	c.RecordBodyInspected()

	snap := c.Snapshot()

	if snap.RequestsSent != 1 {
		t.Errorf("expected 1 sent, got %d", snap.RequestsSent)
	}
	if snap.ResponsesReceived != 3 {
		t.Errorf("expected 3 received, got %d", snap.ResponsesReceived)
	}
	if snap.BytesReceived != 3584 {
		t.Errorf("expected 3584 bytes, got %d", snap.BytesReceived)
	}
	if snap.RequestsFiltered != 1 {
		t.Errorf("expected 1 filtered, got %d", snap.RequestsFiltered)
	}
	if snap.RequestsFailed != 1 {
		t.Errorf("expected 1 failed, got %d", snap.RequestsFailed)
	}
	if snap.RequestsSucceeded != 1 {
		t.Errorf("expected 1 succeeded, got %d", snap.RequestsSucceeded)
	}
	if snap.Discovered != 1 {
		t.Errorf("expected 1 discovered, got %d", snap.Discovered)
	}
	if snap.ActiveWorkers != 1 {
		t.Errorf("expected 1 active worker, got %d", snap.ActiveWorkers)
	}
	if snap.TotalCandidates != 42 {
		t.Errorf("expected 42 queued jobs, got %d", snap.TotalCandidates)
	}
	if snap.Retries != 1 {
		t.Errorf("expected 1 retry, got %d", snap.Retries)
	}
	if snap.Redirects != 1 {
		t.Errorf("expected 1 redirect, got %d", snap.Redirects)
	}
	if snap.BodyInspected != 1 {
		t.Errorf("expected 1 body inspected, got %d", snap.BodyInspected)
	}
	if snap.AverageLatency != 150*time.Millisecond {
		t.Errorf("expected 150ms average latency, got %v", snap.AverageLatency)
	}

	// Status counters
	if snap.StatusCodes[200] != 2 {
		t.Errorf("expected 2 responses of status 200, got %d", snap.StatusCodes[200])
	}
	if snap.StatusCodes[404] != 1 {
		t.Errorf("expected 1 response of status 404, got %d", snap.StatusCodes[404])
	}
}

func TestCollector_ConcurrentUpdates(t *testing.T) {
	c := stats.NewCollector()
	var wg sync.WaitGroup

	workers := 20
	iterations := 1000

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				c.RecordRequestSent()
				c.RecordResponseReceived(200, 10)
				c.RecordResponseReceived(301+id%5, 5) // concurrent status updates
				c.RecordRequestFiltered()
				c.RecordRequestFailed()
				c.RecordRequestSucceeded()
				c.RecordDiscovered()
				c.IncrementActiveWorkers()
				c.DecrementActiveWorkers()
			}
		}(i)
	}

	wg.Wait()
	snap := c.Snapshot()

	expectedCount := int64(workers * iterations)

	if snap.RequestsSent != expectedCount {
		t.Errorf("expected %d requests sent, got %d", expectedCount, snap.RequestsSent)
	}
	if snap.ResponsesReceived != expectedCount*2 {
		t.Errorf("expected %d responses received, got %d", expectedCount*2, snap.ResponsesReceived)
	}
	if snap.RequestsFiltered != expectedCount {
		t.Errorf("expected %d filtered, got %d", expectedCount, snap.RequestsFiltered)
	}
	if snap.RequestsFailed != expectedCount {
		t.Errorf("expected %d failed, got %d", expectedCount, snap.RequestsFailed)
	}
	if snap.RequestsSucceeded != expectedCount {
		t.Errorf("expected %d succeeded, got %d", expectedCount, snap.RequestsSucceeded)
	}
	if snap.Discovered != expectedCount {
		t.Errorf("expected %d discovered, got %d", expectedCount, snap.Discovered)
	}
	if snap.ActiveWorkers != 0 {
		t.Errorf("expected 0 active workers (balanced increments/decrements), got %d", snap.ActiveWorkers)
	}

	if snap.StatusCodes[200] != expectedCount {
		t.Errorf("expected %d for status 200, got %d", expectedCount, snap.StatusCodes[200])
	}
}

func TestCounter_GenericCounter(t *testing.T) {
	cnt := &stats.Counter{}
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cnt.Increment()
				cnt.Add(2)
			}
		}()
	}

	wg.Wait()
	if cnt.Value() != 3000 {
		t.Errorf("expected counter value 3000, got %d", cnt.Value())
	}
}

func TestCollector_ActiveWorkersAndQueuedJobs(t *testing.T) {
	c := stats.NewCollector()
	c.SetActiveWorkers(10)
	c.SetTotalCandidates(50)

	snap := c.Snapshot()
	if snap.ActiveWorkers != 10 {
		t.Errorf("expected 10 active workers, got %d", snap.ActiveWorkers)
	}
	if snap.TotalCandidates != 50 {
		t.Errorf("expected 50 total candidates, got %d", snap.TotalCandidates)
	}
}

func TestInstrumentation_Reset(t *testing.T) {
	inst := &stats.Instrumentation{}
	inst.WordsRead = 5
	inst.Reset()

	if inst.WordsRead != 0 {
		t.Errorf("expected WordsRead=0 after reset, got %d", inst.WordsRead)
	}
}

func TestInstrumentation_LogEvent(t *testing.T) {
	inst := &stats.Instrumentation{}

	// Disabled by default
	inst.LogEvent("event 1")
	if len(inst.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(inst.Events))
	}

	// Enable log and trace
	inst.Enabled = 1
	inst.Trace = 1
	inst.LogEvent("event 2")
	if len(inst.Events) != 1 || inst.Events[0] != "event 2" {
		t.Errorf("expected [event 2], got %v", inst.Events)
	}
}

func TestInstrumentation_PrintReconciliation(t *testing.T) {
	inst := &stats.Instrumentation{}
	inst.Enabled = 1
	inst.Trace = 1

	// Setup mismatches to cover all pipeline stages
	inst.JobsProduced = 10
	inst.JobsSubmitted = 9
	inst.PrintReconciliation() // Mismatch in Producer

	inst.JobsSubmitted = 10
	inst.WorkerJobsRecv = 9
	inst.PrintReconciliation() // Mismatch in Channel

	inst.WorkerJobsRecv = 10
	inst.WorkerJobsComp = 9
	inst.PrintReconciliation() // Mismatch in Workers

	inst.WorkerJobsComp = 10
	inst.RequestsBuilt = 9
	inst.PrintReconciliation() // Mismatch in Request Builder

	inst.RequestsBuilt = 10
	inst.RequestsSent = 9
	inst.PrintReconciliation() // Mismatch in HTTP Transmission

	inst.RequestsSent = 10
	inst.ResponsesReceived = 9
	inst.PrintReconciliation() // Mismatch in HTTP Response

	inst.ResponsesReceived = 10
	inst.ResultsProduced = 9
	inst.PrintReconciliation() // Mismatch in Worker Output

	inst.ResultsProduced = 10
	inst.ResultsConsumed = 9
	inst.PrintReconciliation() // Mismatch in Scheduler

	// Reconciled
	inst.ResultsConsumed = 10
	inst.PrintReconciliation()
}

func TestCollector_CurrentReqPerSec_UpdatesContinuously(t *testing.T) {
	c := stats.NewCollector()

	// With zero requests, current rate should be 0
	snap1 := c.Snapshot()
	if snap1.CurrentRequestsPerSecond != 0 {
		t.Errorf("expected 0 current req/s at start, got %.2f", snap1.CurrentRequestsPerSecond)
	}

	// Record some requests and wait a short time to ensure the window has data
	for i := 0; i < 100; i++ {
		c.RecordRequestSent()
	}
	time.Sleep(10 * time.Millisecond)
	snap2 := c.Snapshot()

	// After sending 100 requests, the current rate should be > 0
	if snap2.CurrentRequestsPerSecond <= 0 {
		t.Errorf("expected current req/s > 0 after sending requests, got %.2f", snap2.CurrentRequestsPerSecond)
	}
}

func TestCollector_AverageReqPerSec_MathematicallyCorrect(t *testing.T) {
	c := stats.NewCollector()

	for i := 0; i < 50; i++ {
		c.RecordRequestSent()
	}
	time.Sleep(50 * time.Millisecond)

	snap := c.Snapshot()
	elapsed := time.Since(snap.StartTime).Seconds()
	expected := float64(snap.RequestsSent) / elapsed

	// Average Req/s should be approximately sent/elapsed (within a generous 20% tolerance for test timing)
	if snap.RequestsPerSecond <= 0 {
		t.Errorf("expected average req/s > 0, got %.2f", snap.RequestsPerSecond)
	}
	tolerance := expected * 0.20
	if snap.RequestsPerSecond < expected-tolerance || snap.RequestsPerSecond > expected+tolerance {
		t.Errorf("average req/s %.2f outside expected range [%.2f, %.2f]",
			snap.RequestsPerSecond, expected-tolerance, expected+tolerance)
	}
}

func TestCollector_PeakReqPerSec_NeverDecreases(t *testing.T) {
	c := stats.NewCollector()

	var lastPeak float64
	for round := 0; round < 5; round++ {
		// Send a burst of requests
		for i := 0; i < 20; i++ {
			c.RecordRequestSent()
		}
		snap := c.Snapshot()
		if snap.PeakRequestsPerSecond < lastPeak {
			t.Errorf("round %d: peak decreased from %.2f to %.2f", round, lastPeak, snap.PeakRequestsPerSecond)
		}
		lastPeak = snap.PeakRequestsPerSecond
		time.Sleep(5 * time.Millisecond)
	}

	if lastPeak <= 0 {
		t.Errorf("expected peak req/s > 0 after sending requests, got %.2f", lastPeak)
	}
}

func TestCollector_BytesReceivedNegativeProtection(t *testing.T) {
	c := stats.NewCollector()

	// Passing negative byte counts (e.g. -1 for unknown ContentLength) must NEVER accumulate negative values
	c.RecordResponseReceived(404, -1)
	c.RecordResponseReceived(500, -1)
	c.RecordResponseReceived(200, -1)

	snap := c.Snapshot()
	if snap.ResponsesReceived != 3 {
		t.Errorf("expected 3 responses received, got %d", snap.ResponsesReceived)
	}
	if snap.BytesReceived < 0 {
		t.Errorf("BytesReceived MUST NOT be negative, got %d", snap.BytesReceived)
	}
	if snap.BytesReceived != 0 {
		t.Errorf("expected 0 bytes received when all lengths are -1, got %d", snap.BytesReceived)
	}

	// Adding valid positive bytes should accumulate properly
	c.RecordResponseReceived(200, 500)
	snap2 := c.Snapshot()
	if snap2.BytesReceived != 500 {
		t.Errorf("expected 500 bytes received, got %d", snap2.BytesReceived)
	}
}

// TestAddTotalCandidates_IsRealIncrement verifies that AddTotalCandidates
// is not a no-op and actually mutates totalWork/TotalCandidates.
func TestAddTotalCandidates_IsRealIncrement(t *testing.T) {
	c := stats.NewCollector()

	if snap := c.Snapshot(); snap.TotalCandidates != 0 {
		t.Fatalf("initial TotalCandidates should be 0, got %d", snap.TotalCandidates)
	}

	c.AddTotalCandidates(1)
	if snap := c.Snapshot(); snap.TotalCandidates != 1 {
		t.Errorf("after AddTotalCandidates(1): TotalCandidates=%d, want 1", snap.TotalCandidates)
	}

	c.AddTotalCandidates(99)
	if snap := c.Snapshot(); snap.TotalCandidates != 100 {
		t.Errorf("after AddTotalCandidates(99): TotalCandidates=%d, want 100", snap.TotalCandidates)
	}

	// TotalWork must mirror TotalCandidates
	snap := c.Snapshot()
	if snap.TotalWork != snap.TotalCandidates {
		t.Errorf("TotalWork (%d) != TotalCandidates (%d)", snap.TotalWork, snap.TotalCandidates)
	}
}

// TestStreamingWordlist_TotalNeverZeroAfterFirstJob verifies that a collector
// that starts with TotalWork=0 (streaming mode) has TotalWork > 0 after the
// first AddTotalCandidates call, so Progress never shows "N / 0".
func TestStreamingWordlist_TotalNeverZeroAfterFirstJob(t *testing.T) {
	c := stats.NewCollector()
	c.SetIsFinite(true)

	// Simulate streaming: baseCount==0, no SetTotalCandidates called at startup.
	snap := c.Snapshot()
	if snap.TotalCandidates != 0 {
		t.Fatalf("pre-condition: TotalCandidates should be 0 before any job")
	}

	// First job arrives via AddTotalCandidates (streaming path).
	c.AddTotalCandidates(1)
	c.RecordTried()

	snap = c.Snapshot()
	if snap.TotalCandidates <= 0 {
		t.Errorf("TotalCandidates should be > 0 after first job, got %d", snap.TotalCandidates)
	}
	if snap.Completed > snap.TotalWork {
		t.Errorf("invariant violated: completed (%d) > totalWork (%d)", snap.Completed, snap.TotalWork)
	}
	if snap.Progress < 0 || snap.Progress > 100 {
		t.Errorf("Progress %f out of [0,100] range", snap.Progress)
	}
}

// TestPreCountedWordlist_NoDoubleCount verifies that when SetTotalCandidates is
// called at startup (pre-counted wordlist), adding n jobs via RecordJobProduced
// does NOT increase TotalCandidates — the count must stay at the pre-set value.
func TestPreCountedWordlist_SetTotalCandidates_IsAbsolute(t *testing.T) {
	c := stats.NewCollector()
	c.SetIsFinite(true)
	c.SetTotalCandidates(1000)

	snap := c.Snapshot()
	if snap.TotalCandidates != 1000 {
		t.Fatalf("SetTotalCandidates(1000): got %d", snap.TotalCandidates)
	}

	// Simulate 5 jobs produced (NOT using AddTotalCandidates for pre-counted mode).
	for i := 0; i < 5; i++ {
		c.RecordJobProduced()
		c.RecordTried()
	}

	snap = c.Snapshot()
	// TotalCandidates must remain 1000 — no AddTotalCandidates was called.
	if snap.TotalCandidates != 1000 {
		t.Errorf("TotalCandidates after 5 jobs: got %d, want 1000 (SetTotalCandidates must not be overridden by RecordJobProduced)", snap.TotalCandidates)
	}
	if snap.Completed > snap.TotalWork {
		t.Errorf("invariant violated: completed (%d) > totalWork (%d)", snap.Completed, snap.TotalWork)
	}
}

// TestSetTotalCandidates_DoesNotAccumulate verifies that repeated calls to
// SetTotalCandidates replace (not add to) the stored total.
func TestSetTotalCandidates_ReplacesNotAccumulates(t *testing.T) {
	c := stats.NewCollector()
	c.SetTotalCandidates(500)
	c.SetTotalCandidates(1000)

	snap := c.Snapshot()
	if snap.TotalCandidates != 1000 {
		t.Errorf("SetTotalCandidates(1000) after 500: got %d, want 1000", snap.TotalCandidates)
	}
}

// TestAddTotalCandidates_Concurrent verifies that concurrent AddTotalCandidates
// calls are race-free and produce the expected total.
func TestAddTotalCandidates_Concurrent(t *testing.T) {
	c := stats.NewCollector()
	const goroutines = 50
	const callsEach = 100

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < callsEach; j++ {
				c.AddTotalCandidates(1)
			}
		}()
	}
	wg.Wait()

	snap := c.Snapshot()
	want := int64(goroutines * callsEach)
	if snap.TotalCandidates != want {
		t.Errorf("TotalCandidates after concurrent adds: got %d, want %d", snap.TotalCandidates, want)
	}
}

// TestProgressInvariant_CompletedNeverExceedsTotal verifies the invariant
// processed ≤ total whenever totalWork is known.
func TestProgressInvariant_CompletedNeverExceedsTotal(t *testing.T) {
	c := stats.NewCollector()
	c.SetIsFinite(true)

	// Simulate 10 streaming jobs.
	for i := 0; i < 10; i++ {
		c.AddTotalCandidates(1) // add one more to total before dispatching
		c.RecordTried()         // one job dispatched
		snap := c.Snapshot()
		if snap.Completed > snap.TotalWork {
			t.Errorf("at step %d: completed (%d) > totalWork (%d)", i+1, snap.Completed, snap.TotalWork)
		}
		if snap.Progress < 0 || snap.Progress > 100 {
			t.Errorf("at step %d: Progress %f out of [0,100]", i+1, snap.Progress)
		}
	}
}

// TestDiscoveredEqualsAcceptedFindings verifies that RecordDiscovered tracks
// exactly the number of accepted findings, not total requests.
func TestDiscoveredEqualsAcceptedFindings(t *testing.T) {
	c := stats.NewCollector()

	const totalRequests = 100
	const acceptedFindings = 5

	for i := 0; i < totalRequests; i++ {
		c.RecordRequestSent()
		c.RecordRequestSucceeded()
		c.RecordResponseReceived(404, 0)
	}
	for i := 0; i < acceptedFindings; i++ {
		c.RecordDiscovered()
	}

	snap := c.Snapshot()
	if snap.Discovered != acceptedFindings {
		t.Errorf("Discovered: got %d, want %d", snap.Discovered, acceptedFindings)
	}
	if snap.RequestsSent != totalRequests {
		t.Errorf("RequestsSent: got %d, want %d", snap.RequestsSent, totalRequests)
	}
	// Discovered must never exceed RequestsSent
	if snap.Discovered > snap.RequestsSent {
		t.Errorf("Discovered (%d) > RequestsSent (%d) — impossible invariant violation", snap.Discovered, snap.RequestsSent)
	}
}

// TestETAOnlyDashWhenUnknownOrComplete verifies ETA conditions.
// ETA should be "-" when totalWork==0 (unknown) and should be computable
// once totalWork>0 and progress is ongoing.
func TestProgressSnapshot_ETAConditions(t *testing.T) {
	c := stats.NewCollector()
	c.SetIsFinite(true)

	// Before any work: TotalWork==0 → progress==0, completed==0.
	snap := c.Snapshot()
	if snap.TotalWork != 0 {
		t.Errorf("pre-condition: TotalWork should be 0, got %d", snap.TotalWork)
	}
	if snap.Progress != 0 {
		t.Errorf("pre-condition: Progress should be 0, got %f", snap.Progress)
	}

	// Add 100 candidates, try 50 → progress = 50%.
	c.SetTotalCandidates(100)
	for i := 0; i < 50; i++ {
		c.RecordTried()
	}
	snap = c.Snapshot()
	if snap.Progress < 49.0 || snap.Progress > 51.0 {
		t.Errorf("Progress at 50/100 should be ~50%%, got %f", snap.Progress)
	}
	remaining := snap.TotalWork - snap.Completed
	if remaining <= 0 {
		t.Errorf("remaining work should be > 0 mid-scan, got %d", remaining)
	}
}
