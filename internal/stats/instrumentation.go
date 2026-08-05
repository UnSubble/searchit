package stats

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

type Instrumentation struct {
	Enabled int32 // 0 = disabled, 1 = enabled
	Trace   int32 // 0 = disabled, 1 = enabled

	// Reader
	WordsRead    int64
	InvalidWords int64
	ReaderEOF    int64
	ReaderExit   int64

	// Producer
	JobsProduced  int64
	JobsSubmitted int64
	ProducerExit  int64

	// Scheduler / Manager
	JobsAccepted   int64
	JobsDispatched int64
	JobsRemaining  int64
	JobsDiscarded  int64
	SchedulerExit  int64

	// Workers
	WorkersStarted int64
	WorkerJobsRecv int64
	WorkerJobsComp int64
	WorkerJobsRej  int64
	WorkersExited  int64
	WorkersActive  int64

	// HTTP Lifecycle
	RequestsBuilt     int64
	RequestsSent      int64
	ResponsesReceived int64

	// Results
	ResultsProduced int64
	ResultsConsumed int64
	ResultsAccepted int64
	ResultsRejected int64

	// Telemetry updates
	WildcardsDetected int64
	RequestsFiltered  int64

	// Shutdown Order Log
	EventsMu sync.Mutex
	Events   []string
}

var GlobalInstrumentation = &Instrumentation{}

func (i *Instrumentation) LogEvent(event string) {
	if atomic.LoadInt32(&i.Enabled) == 0 {
		return
	}
	if atomic.LoadInt32(&i.Trace) == 0 {
		return // Suppress per-event trace logging by default
	}
	i.EventsMu.Lock()
	i.Events = append(i.Events, event)
	i.EventsMu.Unlock()
}

func (i *Instrumentation) PrintReconciliation(w io.Writer) {
	if atomic.LoadInt32(&i.Enabled) == 0 {
		return
	}
	if w == nil {
		w = os.Stderr
	}

	wordsRead := atomic.LoadInt64(&i.WordsRead)
	jobsProduced := atomic.LoadInt64(&i.JobsProduced)
	jobsSubmitted := atomic.LoadInt64(&i.JobsSubmitted)
	jobsRecv := atomic.LoadInt64(&i.WorkerJobsRecv)
	jobsComp := atomic.LoadInt64(&i.WorkerJobsComp)
	reqsBuilt := atomic.LoadInt64(&i.RequestsBuilt)
	reqsSent := atomic.LoadInt64(&i.RequestsSent)
	respsRecv := atomic.LoadInt64(&i.ResponsesReceived)
	resultsProd := atomic.LoadInt64(&i.ResultsProduced)
	resultsCons := atomic.LoadInt64(&i.ResultsConsumed)

	mismatch := false
	mismatchStage := ""

	if jobsSubmitted != jobsProduced {
		mismatch = true
		mismatchStage = "Producer (Jobs Submitted != Jobs Produced)"
	} else if jobsRecv != jobsSubmitted {
		mismatch = true
		mismatchStage = "Channel / Queue (Jobs Received != Jobs Submitted)"
	} else if jobsComp != jobsRecv {
		mismatch = true
		mismatchStage = "Workers (Jobs Completed != Jobs Received)"
	} else if reqsBuilt != jobsComp {
		mismatch = true
		mismatchStage = "Request Builder (Requests Built != Jobs Completed)"
	} else if reqsSent != reqsBuilt {
		mismatch = true
		mismatchStage = "HTTP Transmission (Requests Sent != Requests Built)"
	} else if respsRecv != reqsSent {
		mismatch = true
		mismatchStage = "HTTP Response (Responses Received != Requests Sent)"
	} else if resultsProd != respsRecv {
		// Note: Under normal workflow, some build or HTTP errors might prevent response,
		// but inside workers all jobs must produce a result (either accepted or failed).
		// Thus, resultsProd should equal jobsComp. If not:
		mismatch = true
		mismatchStage = "Worker Output (Results Produced != Responses Received)"
	} else if resultsCons != resultsProd {
		mismatch = true
		mismatchStage = "Scheduler / Draining (Results Consumed != Results Produced)"
	}

	fmt.Fprintf(w, "\n--- PIPELINE RECONCILIATION ---\n")
	fmt.Fprintf(w, "Words Read          : %d\n", wordsRead)
	fmt.Fprintf(w, "Invalid Words       : %d\n", atomic.LoadInt64(&i.InvalidWords))
	fmt.Fprintf(w, "Jobs Produced       : %d\n", jobsProduced)
	fmt.Fprintf(w, "Jobs Submitted      : %d\n", jobsSubmitted)
	fmt.Fprintf(w, "Jobs Received       : %d\n", jobsRecv)
	fmt.Fprintf(w, "Jobs Completed      : %d\n", jobsComp)
	fmt.Fprintf(w, "Requests Built      : %d\n", reqsBuilt)
	fmt.Fprintf(w, "Requests Sent       : %d\n", reqsSent)
	fmt.Fprintf(w, "Responses Received  : %d\n", respsRecv)
	fmt.Fprintf(w, "Results Produced    : %d\n", resultsProd)
	fmt.Fprintf(w, "Results Consumed    : %d\n", resultsCons)
	if mismatch {
		fmt.Fprintf(w, "Status              : ❌ MISMATCH DETECTED (First lost work stage: %s)\n", mismatchStage)
	} else {
		fmt.Fprintf(w, "Status              :  Reconciled\n")
	}

	if atomic.LoadInt32(&i.Trace) != 0 {
		fmt.Fprintf(w, "\n--- DETAILED COUNTERS ---\n")
		fmt.Fprintf(w, "Reader EOF Reached  : %d\n", atomic.LoadInt64(&i.ReaderEOF))
		fmt.Fprintf(w, "Reader Exited       : %d\n", atomic.LoadInt64(&i.ReaderExit))
		fmt.Fprintf(w, "Producer Exited     : %d\n", atomic.LoadInt64(&i.ProducerExit))
		fmt.Fprintf(w, "Scheduler Accepted  : %d\n", atomic.LoadInt64(&i.JobsAccepted))
		fmt.Fprintf(w, "Scheduler Dispatched: %d\n", atomic.LoadInt64(&i.JobsDispatched))
		fmt.Fprintf(w, "Scheduler Remaining : %d\n", atomic.LoadInt64(&i.JobsRemaining))
		fmt.Fprintf(w, "Scheduler Discarded : %d\n", atomic.LoadInt64(&i.JobsDiscarded))
		fmt.Fprintf(w, "Scheduler Exited    : %d\n", atomic.LoadInt64(&i.SchedulerExit))
		fmt.Fprintf(w, "Workers Started     : %d\n", atomic.LoadInt64(&i.WorkersStarted))
		fmt.Fprintf(w, "Workers Exited      : %d\n", atomic.LoadInt64(&i.WorkersExited))
		fmt.Fprintf(w, "Worker Jobs Rejected: %d\n", atomic.LoadInt64(&i.WorkerJobsRej))
		fmt.Fprintf(w, "Results Accepted    : %d\n", atomic.LoadInt64(&i.ResultsAccepted))
		fmt.Fprintf(w, "Results Rejected    : %d\n", atomic.LoadInt64(&i.ResultsRejected))
		fmt.Fprintf(w, "\n--- SHUTDOWN ORDER ---\n")
		i.EventsMu.Lock()
		for idx, ev := range i.Events {
			fmt.Fprintf(w, "  %d. %s\n", idx+1, ev)
		}
		i.EventsMu.Unlock()
	}
	fmt.Fprintf(w, "-------------------------------\n\n")
}

func (i *Instrumentation) Reset() {
	atomic.StoreInt32(&i.Trace, 0)
	atomic.StoreInt64(&i.WordsRead, 0)
	atomic.StoreInt64(&i.InvalidWords, 0)
	atomic.StoreInt64(&i.ReaderEOF, 0)
	atomic.StoreInt64(&i.ReaderExit, 0)
	atomic.StoreInt64(&i.JobsProduced, 0)
	atomic.StoreInt64(&i.JobsSubmitted, 0)
	atomic.StoreInt64(&i.ProducerExit, 0)
	atomic.StoreInt64(&i.JobsAccepted, 0)
	atomic.StoreInt64(&i.JobsDispatched, 0)
	atomic.StoreInt64(&i.JobsRemaining, 0)
	atomic.StoreInt64(&i.JobsDiscarded, 0)
	atomic.StoreInt64(&i.SchedulerExit, 0)
	atomic.StoreInt64(&i.WorkersStarted, 0)
	atomic.StoreInt64(&i.WorkerJobsRecv, 0)
	atomic.StoreInt64(&i.WorkerJobsComp, 0)
	atomic.StoreInt64(&i.WorkerJobsRej, 0)
	atomic.StoreInt64(&i.WorkersExited, 0)
	atomic.StoreInt64(&i.RequestsBuilt, 0)
	atomic.StoreInt64(&i.RequestsSent, 0)
	atomic.StoreInt64(&i.ResponsesReceived, 0)
	atomic.StoreInt64(&i.ResultsProduced, 0)
	atomic.StoreInt64(&i.ResultsConsumed, 0)
	atomic.StoreInt64(&i.ResultsAccepted, 0)
	atomic.StoreInt64(&i.ResultsRejected, 0)
	atomic.StoreInt64(&i.WildcardsDetected, 0)
	atomic.StoreInt64(&i.RequestsFiltered, 0)
	i.EventsMu.Lock()
	i.Events = nil
	i.EventsMu.Unlock()
}
