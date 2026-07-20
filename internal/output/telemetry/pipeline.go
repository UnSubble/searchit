package telemetry

import (
	"fmt"
	"io"
	"sync/atomic"

	"github.com/unsubble/searchit/internal/stats"
)

func PrintPipelineReconciliation(w io.Writer) {
	inst := stats.GlobalInstrumentation
	if atomic.LoadInt32(&inst.Enabled) == 0 {
		return
	}

	wordsRead := atomic.LoadInt64(&inst.WordsRead)
	jobsProduced := atomic.LoadInt64(&inst.JobsProduced)
	jobsSubmitted := atomic.LoadInt64(&inst.JobsSubmitted)
	jobsRecv := atomic.LoadInt64(&inst.WorkerJobsRecv)
	jobsComp := atomic.LoadInt64(&inst.WorkerJobsComp)
	reqsBuilt := atomic.LoadInt64(&inst.RequestsBuilt)
	reqsSent := atomic.LoadInt64(&inst.RequestsSent)
	respsRecv := atomic.LoadInt64(&inst.ResponsesReceived)
	resultsProd := atomic.LoadInt64(&inst.ResultsProduced)
	resultsCons := atomic.LoadInt64(&inst.ResultsConsumed)

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
		mismatch = true
		mismatchStage = "Worker Output (Results Produced != Responses Received)"
	} else if resultsCons != resultsProd {
		mismatch = true
		mismatchStage = "Scheduler / Draining (Results Consumed != Results Produced)"
	}

	statusStr := "Reconciled"
	if mismatch {
		statusStr = fmt.Sprintf("Mismatch Detected (First lost work stage: %s)", mismatchStage)
	}

	fmt.Fprintln(w, "---------------------------------------------------------")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "PIPELINE RECONCILIATION")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Candidates Read:\n    %d\n\n", wordsRead)
	fmt.Fprintf(w, "Jobs Produced:\n    %d\n\n", jobsProduced)
	fmt.Fprintf(w, "Requests Sent:\n    %d\n\n", reqsSent)
	fmt.Fprintf(w, "Responses Received:\n    %d\n\n", respsRecv)
	fmt.Fprintf(w, "Results Produced:\n    %d\n\n", resultsProd)
	fmt.Fprintf(w, "Results Consumed:\n    %d\n\n", resultsCons)

	fmt.Fprintf(w, "Status:\n    %s\n\n", statusStr)
	fmt.Fprintln(w, "---------------------------------------------------------")
}
