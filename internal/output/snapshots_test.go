package output_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/output/telemetry"
	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/stats"
)

func TestUnifiedDesignLanguageConsistency(t *testing.T) {
	// Verify that Scan and Fuzz configurations share identical separator and block structures
	var scanBuf, fuzzBuf bytes.Buffer

	scanTm := terminal.New(&scanBuf)
	_ = scanTm.AcquireOwner(terminal.OwnerConfiguration)
	telemetry.PrintConfiguration(scanTm, terminal.OwnerConfiguration, telemetry.ConfigInfo{
		Target:          "http://127.0.0.1:8080",
		Method:          "GET",
		Workers:         32,
		Strategy:        "bfs",
		AdaptiveEnabled: false,
		PrimaryWordlist: "words.txt",
		IsFuzz:          false,
	})

	fuzzTm := terminal.New(&fuzzBuf)
	_ = fuzzTm.AcquireOwner(terminal.OwnerConfiguration)
	telemetry.PrintConfiguration(fuzzTm, terminal.OwnerConfiguration, telemetry.ConfigInfo{
		Target:          "http://127.0.0.1:8080/FUZZ",
		Method:          "GET",
		Workers:         32,
		Strategy:        "bfs",
		AdaptiveEnabled: false,
		PrimaryWordlist: "words.txt",
		IsFuzz:          true,
	})
	got := scanBuf.String()
	if !strings.Contains(got, "Target .................... http://127.0.0.1:8080") {
		t.Errorf("Scan snapshot target line mismatch:\n%s", got)
	}
}

func TestSnapshot_Scan(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerConfiguration)
	telemetry.PrintConfiguration(tm, terminal.OwnerConfiguration, telemetry.ConfigInfo{
		Target:          "http://127.0.0.1:8080",
		Method:          "GET",
		Workers:         32,
		Strategy:        "bfs",
		AdaptiveEnabled: false,
		PrimaryWordlist: "words.txt",
		IsFuzz:          false,
	})

	got := buf.String()
	if !strings.Contains(got, "SCAN CONFIGURATION") {
		t.Errorf("Scan snapshot header missing:\n%s", got)
	}
	if !strings.Contains(got, "Target .................... http://127.0.0.1:8080") {
		t.Errorf("Scan snapshot target line mismatch:\n%s", got)
	}
}

func TestSnapshot_Fuzz(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerConfiguration)
	telemetry.PrintConfiguration(tm, terminal.OwnerConfiguration, telemetry.ConfigInfo{
		Target:          "http://127.0.0.1:8080/api?user=FUZZ",
		Method:          "POST",
		Workers:         32,
		Strategy:        "bfs",
		AdaptiveEnabled: false,
		WordlistsCount:  1,
		PrimaryWordlist: "users.txt",
		Placeholders:    "FUZZ (1)",
		IsFuzz:          true,
	})

	got := buf.String()
	if !strings.Contains(got, "FUZZ CONFIGURATION") {
		t.Errorf("Fuzz snapshot header missing:\n%s", got)
	}
	if !strings.Contains(got, "Primary Wordlist .......... users.txt") {
		t.Errorf("Fuzz snapshot wordlist line mismatch:\n%s", got)
	}
}

func TestSnapshot_Recursive(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerConfiguration)
	telemetry.PrintConfiguration(tm, terminal.OwnerConfiguration, telemetry.ConfigInfo{
		Target:          "http://127.0.0.1:8080",
		Method:          "GET",
		Workers:         64,
		Strategy:        "recursive (BFS depth 3)",
		AdaptiveEnabled: false,
		PrimaryWordlist: "words.txt",
		IsFuzz:          false,
	})

	if !strings.Contains(buf.String(), "Strategy .................. recursive (BFS depth 3)") {
		t.Errorf("Recursive snapshot strategy line mismatch:\n%s", buf.String())
	}
}

func TestSnapshot_Adaptive(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerPipeline)
	telemetry.PrintAdaptive(tm, terminal.OwnerPipeline, telemetry.AdaptiveInfo{
		Technologies:        []string{"Laravel", "WordPress"},
		Discoveries:         []string{"robots.txt", "sitemap.xml"},
		DFSCount:            10,
		BFSCount:            25,
		EagerCount:          3,
		HighPriorityCount:   30,
		MediumPriorityCount: 5,
		LowPriorityCount:    500,
	})

	got := buf.String()
	if !strings.Contains(got, "ADAPTIVE SUMMARY") {
		t.Errorf("Adaptive snapshot header missing:\n%s", got)
	}
	if !strings.Contains(got, "Technologies .............. Laravel, WordPress") {
		t.Errorf("Adaptive snapshot technologies line mismatch:\n%s", got)
	}
}

func TestSnapshot_SmartStrategy(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerConfiguration)
	telemetry.PrintConfiguration(tm, terminal.OwnerConfiguration, telemetry.ConfigInfo{
		Target:          "http://127.0.0.1:8080/FUZZ",
		Method:          "GET",
		Workers:         32,
		Strategy:        "smart",
		AdaptiveEnabled: true,
		WordlistsCount:  1,
		PrimaryWordlist: "words.txt",
		Placeholders:    "FUZZ",
		IsFuzz:          true,
	})

	if !strings.Contains(buf.String(), "Strategy .................. smart") {
		t.Errorf("Smart strategy snapshot mismatch:\n%s", buf.String())
	}
}

func TestSnapshot_Profiles(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerConfiguration)
	telemetry.PrintConfiguration(tm, terminal.OwnerConfiguration, telemetry.ConfigInfo{
		Target:          "http://127.0.0.1:8080",
		Method:          "GET",
		Workers:         32,
		Strategy:        "scan-extra/laravel (PHP)",
		AdaptiveEnabled: true,
		PrimaryWordlist: "embedded",
		IsFuzz:          false,
	})

	if !strings.Contains(buf.String(), "Strategy .................. scan-extra/laravel (PHP)") {
		t.Errorf("Profiles snapshot strategy line mismatch:\n%s", buf.String())
	}
}

func TestSnapshot_RequestTemplates(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerConfiguration)
	telemetry.PrintConfiguration(tm, terminal.OwnerConfiguration, telemetry.ConfigInfo{
		Target:          "http://127.0.0.1:8080/api/v1",
		Method:          "POST",
		Workers:         32,
		Strategy:        "template (request.txt)",
		AdaptiveEnabled: false,
		PrimaryWordlist: "words.txt",
		IsFuzz:          true,
	})

	if !strings.Contains(buf.String(), "Strategy .................. template (request.txt)") {
		t.Errorf("Request templates snapshot strategy line mismatch:\n%s", buf.String())
	}
}

func TestSnapshot_JSONFormatter(t *testing.T) {
	var buf bytes.Buffer
	jf := output.NewJSONFormatter(&buf, true, true)

	res := engine.Result{
		URL:        "http://127.0.0.1:8080/admin",
		StatusCode: 200,
		Length:     1024,
		Title:      "Admin Login",
	}

	if err := jf.Print(res); err != nil {
		t.Fatalf("JSON Print failed: %v", err)
	}
	if err := jf.Close(); err != nil {
		t.Fatalf("JSON Close failed: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `"url": "http://127.0.0.1:8080/admin"`) || !strings.Contains(got, `"title": "Admin Login"`) {
		t.Errorf("JSON Formatter snapshot mismatch:\n%s", got)
	}
}

func TestSnapshot_MarkdownFormatter(t *testing.T) {
	var buf bytes.Buffer
	mf := output.NewMarkdownFormatter(&buf, false, true)

	res := engine.Result{
		URL:        "http://127.0.0.1:8080/admin",
		StatusCode: 200,
		Length:     1024,
		Title:      "Admin Login",
	}

	if err := mf.Print(res); err != nil {
		t.Fatalf("Markdown Print failed: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "| URL | Status | Length | Depth | Title |") {
		t.Errorf("Markdown Formatter snapshot header mismatch:\n%s", got)
	}
	if !strings.Contains(got, "| http://127.0.0.1:8080/admin | 200 | 1024 | 0 | Admin Login |") {
		t.Errorf("Markdown Formatter snapshot data row mismatch:\n%s", got)
	}
}

func TestSnapshot_SummaryFormat(t *testing.T) {
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	_ = tm.AcquireOwner(terminal.OwnerSummary)
	telemetry.PrintSummary(tm, terminal.OwnerSummary, telemetry.SummaryInfo{
		IsFuzz:          false,
		Strategy:        "bfs",
		AdaptiveEnabled: false,
		Findings:        451,
		Snapshot: stats.Snapshot{
			StartTime:         time.Now().Add(-520 * time.Millisecond),
			RequestsSent:      54123,
			ResponsesReceived: 54123,
		},
	}, false)

	got := buf.String()
	if !strings.Contains(got, "SCAN SUMMARY") {
		t.Errorf("Summary snapshot header missing:\n%s", got)
	}
	if !strings.Contains(buf.String(), "Findings .................. 451") {
		t.Errorf("Summary snapshot findings line mismatch:\n%s", got)
	}
}
