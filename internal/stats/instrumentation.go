package stats

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

type Instrumentation struct {
	Enabled int32 // 0 = disabled, 1 = enabled
	Trace   int32 // 0 = disabled, 1 = enabled

	// Reader
	WordsRead  int64
	ReaderEOF  int64
	ReaderExit int64

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

	// Results
	ResultsProduced int64
	ResultsConsumed int64
	ResultsAccepted int64
	ResultsRejected int64

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

func (i *Instrumentation) PrintReconciliation() {
	if atomic.LoadInt32(&i.Enabled) == 0 {
		return
	}

	wordsRead := atomic.LoadInt64(&i.WordsRead)
	jobsProduced := atomic.LoadInt64(&i.JobsProduced)
	jobsSubmitted := atomic.LoadInt64(&i.JobsSubmitted)
	jobsRecv := atomic.LoadInt64(&i.WorkerJobsRecv)
	jobsComp := atomic.LoadInt64(&i.WorkerJobsComp)
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
	} else if resultsProd != jobsComp {
		mismatch = true
		mismatchStage = "Worker Output (Results Produced != Jobs Completed)"
	} else if resultsCons != resultsProd {
		mismatch = true
		mismatchStage = "Scheduler / Draining (Results Consumed != Results Produced)"
	}

	fmt.Fprintf(os.Stderr, "\n--- PIPELINE RECONCILIATION ---\n")
	fmt.Fprintf(os.Stderr, "Words Read          : %d\n", wordsRead)
	fmt.Fprintf(os.Stderr, "Jobs Produced       : %d\n", jobsProduced)
	fmt.Fprintf(os.Stderr, "Jobs Submitted      : %d\n", jobsSubmitted)
	fmt.Fprintf(os.Stderr, "Jobs Received       : %d\n", jobsRecv)
	fmt.Fprintf(os.Stderr, "Jobs Completed      : %d\n", jobsComp)
	fmt.Fprintf(os.Stderr, "Results Produced    : %d\n", resultsProd)
	fmt.Fprintf(os.Stderr, "Results Consumed    : %d\n", resultsCons)
	if mismatch {
		fmt.Fprintf(os.Stderr, "Status              : ❌ MISMATCH DETECTED (First lost work stage: %s)\n", mismatchStage)
	} else {
		fmt.Fprintf(os.Stderr, "Status              :  Reconciled\n")
	}

	if atomic.LoadInt32(&i.Trace) != 0 {
		fmt.Fprintf(os.Stderr, "\n--- DETAILED COUNTERS ---\n")
		fmt.Fprintf(os.Stderr, "Reader EOF Reached  : %d\n", atomic.LoadInt64(&i.ReaderEOF))
		fmt.Fprintf(os.Stderr, "Reader Exited       : %d\n", atomic.LoadInt64(&i.ReaderExit))
		fmt.Fprintf(os.Stderr, "Producer Exited     : %d\n", atomic.LoadInt64(&i.ProducerExit))
		fmt.Fprintf(os.Stderr, "Scheduler Accepted  : %d\n", atomic.LoadInt64(&i.JobsAccepted))
		fmt.Fprintf(os.Stderr, "Scheduler Dispatched: %d\n", atomic.LoadInt64(&i.JobsDispatched))
		fmt.Fprintf(os.Stderr, "Scheduler Remaining : %d\n", atomic.LoadInt64(&i.JobsRemaining))
		fmt.Fprintf(os.Stderr, "Scheduler Discarded : %d\n", atomic.LoadInt64(&i.JobsDiscarded))
		fmt.Fprintf(os.Stderr, "Scheduler Exited    : %d\n", atomic.LoadInt64(&i.SchedulerExit))
		fmt.Fprintf(os.Stderr, "Workers Started     : %d\n", atomic.LoadInt64(&i.WorkersStarted))
		fmt.Fprintf(os.Stderr, "Workers Exited      : %d\n", atomic.LoadInt64(&i.WorkersExited))
		fmt.Fprintf(os.Stderr, "Worker Jobs Rejected: %d\n", atomic.LoadInt64(&i.WorkerJobsRej))
		fmt.Fprintf(os.Stderr, "Results Accepted    : %d\n", atomic.LoadInt64(&i.ResultsAccepted))
		fmt.Fprintf(os.Stderr, "Results Rejected    : %d\n", atomic.LoadInt64(&i.ResultsRejected))
		fmt.Fprintf(os.Stderr, "\n--- SHUTDOWN ORDER ---\n")
		i.EventsMu.Lock()
		for idx, ev := range i.Events {
			fmt.Fprintf(os.Stderr, "  %d. %s\n", idx+1, ev)
		}
		i.EventsMu.Unlock()
	}
	fmt.Fprintf(os.Stderr, "-------------------------------\n\n")
}

func (i *Instrumentation) Reset() {
	atomic.StoreInt32(&i.Trace, 0)
	atomic.StoreInt64(&i.WordsRead, 0)
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
	atomic.StoreInt64(&i.ResultsProduced, 0)
	atomic.StoreInt64(&i.ResultsConsumed, 0)
	atomic.StoreInt64(&i.ResultsAccepted, 0)
	atomic.StoreInt64(&i.ResultsRejected, 0)
	i.EventsMu.Lock()
	i.Events = nil
	i.EventsMu.Unlock()
}
