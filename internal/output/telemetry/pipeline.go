package telemetry

import (
	"fmt"
	"io"
	"strconv"
	"sync/atomic"

	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/stats"
)

// PrintPipelineReconciliation prints the pipeline reconciliation block.
// All output is routed through tm.Emit(owner, fn).
func PrintPipelineReconciliation(tm *terminal.Manager, owner terminal.Owner) {
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
	wildcards := atomic.LoadInt64(&inst.WildcardsDetected)
	reqsFiltered := atomic.LoadInt64(&inst.RequestsFiltered)

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
		statusStr = fmt.Sprintf("Mismatch Detected (%s)", mismatchStage)
	}

	items := []terminal.Item{
		{Key: "Candidates Read", Value: strconv.FormatInt(wordsRead, 10)},
		{Key: "Jobs Produced", Value: strconv.FormatInt(jobsProduced, 10)},
		{Key: "Jobs Submitted", Value: strconv.FormatInt(jobsSubmitted, 10)},
		{Key: "Jobs Received", Value: strconv.FormatInt(jobsRecv, 10)},
		{Key: "Jobs Completed", Value: strconv.FormatInt(jobsComp, 10)},
		{Key: "Requests Built", Value: strconv.FormatInt(reqsBuilt, 10)},
		{Key: "Requests Sent", Value: strconv.FormatInt(reqsSent, 10)},
		{Key: "Responses Received", Value: strconv.FormatInt(respsRecv, 10)},
		{Key: "Results Produced", Value: strconv.FormatInt(resultsProd, 10)},
		{Key: "Results Consumed", Value: strconv.FormatInt(resultsCons, 10)},
		{Key: "Wildcards Detected", Value: strconv.FormatInt(wildcards, 10)},
		{Key: "Wildcard Filtered", Value: strconv.FormatInt(reqsFiltered, 10)},
		{Key: "Status", Value: statusStr},
	}

	_ = tm.Emit(owner, func(w io.Writer) {
		terminal.RenderBlock(w, "PIPELINE RECONCILIATION SUMMARY", items, tm.ContentWidth())
	})
}
