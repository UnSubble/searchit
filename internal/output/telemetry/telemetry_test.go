package telemetry_test

import (
	"bytes"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/output/telemetry"
	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/stats"
)

func newTestTerminalManager(buf *bytes.Buffer) *terminal.Manager {
	return terminal.New(buf)
}

func advanceToSummary(tm *terminal.Manager) {
	_ = tm.TransitionTo(terminal.PhaseRunning)
	_ = tm.TransitionTo(terminal.PhaseWaitingWorkers)
	_ = tm.TransitionTo(terminal.PhaseFinalizing)
	_ = tm.TransitionTo(terminal.PhaseTerminalShutdown)
	_ = tm.AcquireAndTransition(terminal.OwnerSummary, terminal.PhaseSummary)
}

func advanceToPipeline(tm *terminal.Manager) {
	advanceToSummary(tm)
	_ = tm.ReleaseOwner(terminal.OwnerSummary)
	_ = tm.AcquireAndTransition(terminal.OwnerPipeline, terminal.PhasePipeline)
}

func TestPrintSummary_ScanAndFuzz(t *testing.T) {
	tests := []struct {
		name      string
		info      telemetry.SummaryInfo
		debugMode bool
		wantTitle string
		wantTexts []string
	}{
		{
			name: "Scan Summary Standard",
			info: telemetry.SummaryInfo{
				IsFuzz:   false,
				Findings: 5,
				Snapshot: stats.Snapshot{
					StartTime:    time.Now().Add(-2 * time.Second),
					JobsProduced: 100,
					RequestsSent: 100,
					InvalidWords: 2,
				},
			},
			debugMode: false,
			wantTitle: "SCAN SUMMARY",
			wantTexts: []string{"Candidates", "100", "Findings", "5", "Invalid Words", "2"},
		},
		{
			name: "Fuzz Summary with Debug",
			info: telemetry.SummaryInfo{
				IsFuzz:          true,
				Mode:            "Fuzz",
				Traversal:       "EAGER",
				AdaptiveEnabled: true,
				Findings:        12,
				Snapshot: stats.Snapshot{
					StartTime:         time.Now().Add(-1 * time.Second),
					JobsProduced:      0,
					RequestsSent:      50,
					ResponsesReceived: 50,
					StatusCodes:       map[int]int64{200: 10, 404: 40},
					RequestsFiltered:  40,
				},
			},
			debugMode: true,
			wantTitle: "FUZZ SUMMARY",
			wantTexts: []string{"Candidates", "50", "Findings", "12", "Adaptive", "enabled", "Status 200", "10", "Status 404", "40", "Requests Filtered", "40"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			tm := newTestTerminalManager(&buf)
			advanceToSummary(tm)

			telemetry.PrintSummary(tm, terminal.OwnerSummary, tc.info, tc.debugMode)

			out := buf.String()
			if !strings.Contains(out, tc.wantTitle) {
				t.Errorf("expected title %q, got output:\n%s", tc.wantTitle, out)
			}
			for _, text := range tc.wantTexts {
				if !strings.Contains(out, text) {
					t.Errorf("expected output to contain %q, got:\n%s", text, out)
				}
			}
		})
	}
}

func TestPrintAdaptive(t *testing.T) {
	var buf bytes.Buffer
	tm := newTestTerminalManager(&buf)
	advanceToPipeline(tm)

	info := telemetry.AdaptiveInfo{
		Technologies:        []string{"laravel", "php"},
		Discoveries:         []string{"/api", "/admin"},
		DFSCount:            1,
		BFSCount:            2,
		EagerCount:          3,
		HighPriorityCount:   4,
		MediumPriorityCount: 5,
		LowPriorityCount:    6,
	}

	telemetry.PrintAdaptive(tm, terminal.OwnerPipeline, info)

	out := buf.String()
	if !strings.Contains(out, "ADAPTIVE SUMMARY") {
		t.Fatalf("expected ADAPTIVE SUMMARY header, got:\n%s", out)
	}
	if !strings.Contains(out, "laravel, php") || !strings.Contains(out, "/api, /admin") {
		t.Errorf("expected technologies and discoveries, got:\n%s", out)
	}
}

func TestPrintAdaptive_Empty(t *testing.T) {
	var buf bytes.Buffer
	tm := newTestTerminalManager(&buf)
	advanceToPipeline(tm)

	telemetry.PrintAdaptive(tm, terminal.OwnerPipeline, telemetry.AdaptiveInfo{})

	out := buf.String()
	if !strings.Contains(out, "None detected") || !strings.Contains(out, "None") {
		t.Errorf("expected None placeholders for empty adaptive info, got:\n%s", out)
	}
}

func TestPrintConfiguration_Variants(t *testing.T) {
	var buf bytes.Buffer
	tm := newTestTerminalManager(&buf)
	_ = tm.AcquireOwner(terminal.OwnerConfiguration)

	cfg := telemetry.ConfigInfo{
		Target:          "http://example.com/FUZZ",
		Method:          "POST",
		Workers:         16,
		Mode:            "Standard",
		Traversal:       "BFS",
		AdaptiveEnabled: true,
		WordlistsCount:  2,
		PrimaryWordlist: "custom.txt",
		Placeholders:    "FUZZ (1), FOO (2)",
		HTTPVersion:     "2",
		FollowRedirects: true,
		IsFuzz:          true,
		Extensions:      []string{".php", ".html"},
	}

	telemetry.PrintConfiguration(tm, terminal.OwnerConfiguration, cfg)
	telemetry.PrintNormalConfiguration(tm, terminal.OwnerConfiguration, cfg)

	out := buf.String()
	if !strings.Contains(out, "http://example.com/FUZZ") {
		t.Errorf("expected target URL, got:\n%s", out)
	}
	if !strings.Contains(out, "POST") || !strings.Contains(out, "HTTP Version") {
		t.Errorf("expected HTTP details in full config, got:\n%s", out)
	}
}

func TestPrintPipelineReconciliation(t *testing.T) {
	inst := stats.GlobalInstrumentation
	atomic.StoreInt32(&inst.Enabled, 0)

	var buf bytes.Buffer
	tm := newTestTerminalManager(&buf)
	advanceToPipeline(tm)

	// Disabled does nothing
	telemetry.PrintPipelineReconciliation(tm, terminal.OwnerPipeline)
	if buf.Len() > 0 {
		t.Errorf("expected no output when instrumentation disabled, got:\n%s", buf.String())
	}

	// Enabled reconciled
	atomic.StoreInt32(&inst.Enabled, 1)
	atomic.StoreInt64(&inst.JobsProduced, 10)
	atomic.StoreInt64(&inst.JobsSubmitted, 10)
	atomic.StoreInt64(&inst.WorkerJobsRecv, 10)
	atomic.StoreInt64(&inst.WorkerJobsComp, 10)
	atomic.StoreInt64(&inst.RequestsBuilt, 10)
	atomic.StoreInt64(&inst.RequestsSent, 10)
	atomic.StoreInt64(&inst.ResponsesReceived, 10)
	atomic.StoreInt64(&inst.ResultsProduced, 10)
	atomic.StoreInt64(&inst.ResultsConsumed, 10)

	buf.Reset()
	telemetry.PrintPipelineReconciliation(tm, terminal.OwnerPipeline)
	out := buf.String()
	if !strings.Contains(out, "PIPELINE RECONCILIATION SUMMARY") || !strings.Contains(out, "Reconciled") {
		t.Errorf("expected Reconciled status in output, got:\n%s", out)
	}

	// Mismatch detected
	atomic.StoreInt64(&inst.JobsSubmitted, 9)
	buf.Reset()
	telemetry.PrintPipelineReconciliation(tm, terminal.OwnerPipeline)
	outMismatch := buf.String()
	if !strings.Contains(outMismatch, "Mismatch Detected (Producer (Jobs Submitted != Jobs Produced))") {
		t.Errorf("expected mismatch status in output, got:\n%s", outMismatch)
	}
}

func TestPerformance(t *testing.T) {
	info := telemetry.PerformanceInfo{
		StartTime:    time.Now().Add(-1 * time.Second),
		RequestsSent: 500,
	}

	items := telemetry.GetPerformanceItems(info)
	if len(items) != 2 {
		t.Fatalf("expected 2 performance items, got %d", len(items))
	}

	var buf bytes.Buffer
	telemetry.PrintPerformance(&buf, info)
	if !strings.Contains(buf.String(), "Req/sec") {
		t.Errorf("expected Req/sec in performance output, got:\n%s", buf.String())
	}
}
