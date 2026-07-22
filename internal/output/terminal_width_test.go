package output_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/output/telemetry"
	"github.com/unsubble/searchit/internal/stats"
)

func TestTerminalWidthConstraints(t *testing.T) {
	widths := []struct {
		cols int
		rows int
		name string
	}{
		{cols: 80, rows: 24, name: "80x24"},
		{cols: 100, rows: 25, name: "100x25"},
		{cols: 120, rows: 30, name: "120x30"},
		{cols: 160, rows: 40, name: "160x40"},
		{cols: 200, rows: 50, name: "200x50"},
	}

	scenarios := []struct {
		name   string
		render func(w *bytes.Buffer, width int)
	}{
		{
			name: "Scan Configuration",
			render: func(w *bytes.Buffer, width int) {
				telemetry.PrintConfigurationWithWidth(w, telemetry.ConfigInfo{
					Target:          "http://127.0.0.1:8080/deep/path/to/target/application/endpoint",
					Method:          "GET",
					Workers:         64,
					Strategy:        "recursive",
					AdaptiveEnabled: true,
					PrimaryWordlist: "words.txt",
					IsFuzz:          false,
				}, width)
			},
		},
		{
			name: "Fuzz Configuration",
			render: func(w *bytes.Buffer, width int) {
				telemetry.PrintConfigurationWithWidth(w, telemetry.ConfigInfo{
					Target:          "http://127.0.0.1:8080/api?user=FUZZ&type=FOO",
					Method:          "POST",
					Workers:         32,
					Strategy:        "smart",
					AdaptiveEnabled: false,
					WordlistsCount:  2,
					PrimaryWordlist: "users.txt",
					Placeholders:    "FUZZ, FOO",
					IsFuzz:          true,
				}, width)
			},
		},
		{
			name: "Scan Summary",
			render: func(w *bytes.Buffer, width int) {
				telemetry.PrintSummaryWithWidth(w, telemetry.SummaryInfo{
					IsFuzz:          false,
					Strategy:        "bfs",
					AdaptiveEnabled: true,
					Candidates:      125412,
					Findings:        451,
					Snapshot: stats.Snapshot{
						StartTime:         time.Now().Add(-520 * time.Millisecond),
						RequestsSent:      54123,
						ResponsesReceived: 54123,
					},
				}, true, width)
			},
		},
		{
			name: "Fuzz Summary",
			render: func(w *bytes.Buffer, width int) {
				telemetry.PrintSummaryWithWidth(w, telemetry.SummaryInfo{
					IsFuzz:          true,
					Strategy:        "smart",
					AdaptiveEnabled: false,
					Candidates:      50000,
					Findings:        120,
					Snapshot: stats.Snapshot{
						StartTime:         time.Now().Add(-1100 * time.Millisecond),
						RequestsSent:      45454,
						ResponsesReceived: 45454,
					},
				}, false, width)
			},
		},
		{
			name: "Adaptive Summary",
			render: func(w *bytes.Buffer, width int) {
				telemetry.PrintAdaptiveWithWidth(w, telemetry.AdaptiveInfo{
					Technologies:        []string{"Laravel", "WordPress"},
					Discoveries:         []string{"robots.txt", "sitemap.xml"},
					DFSCount:            12,
					BFSCount:            30,
					EagerCount:          5,
					HighPriorityCount:   42,
					MediumPriorityCount: 10,
					LowPriorityCount:    1200,
				}, width)
			},
		},
		{
			name: "Dot Padded Row",
			render: func(w *bytes.Buffer, width int) {
				row := output.FormatRow("Very Long Category Key Name That Might Be Printed", "http://127.0.0.1:8080/very/long/url/value/path", width)
				w.WriteString(row + "\n")
			},
		},
	}

	for _, dim := range widths {
		t.Run(dim.name, func(t *testing.T) {
			for _, sc := range scenarios {
				t.Run(sc.name, func(t *testing.T) {
					var buf bytes.Buffer
					sc.render(&buf, dim.cols)

					lines := strings.Split(buf.String(), "\n")
					for lineNo, line := range lines {
						if len(line) > dim.cols {
							t.Fatalf("Line %d exceeded terminal width %d (got %d chars):\n%s",
								lineNo+1, dim.cols, len(line), line)
						}
					}
				})
			}
		})
	}
}

func TestUnifiedDesignLanguageConsistency(t *testing.T) {
	// Verify that Scan and Fuzz configurations share identical separator and block structures
	var scanBuf, fuzzBuf bytes.Buffer

	telemetry.PrintConfiguration(&scanBuf, telemetry.ConfigInfo{
		Target:          "http://127.0.0.1:8080",
		Method:          "GET",
		Workers:         32,
		Strategy:        "bfs",
		AdaptiveEnabled: false,
		PrimaryWordlist: "words.txt",
		IsFuzz:          false,
	})

	telemetry.PrintConfiguration(&fuzzBuf, telemetry.ConfigInfo{
		Target:          "http://127.0.0.1:8080/FUZZ",
		Method:          "GET",
		Workers:         32,
		Strategy:        "bfs",
		AdaptiveEnabled: false,
		PrimaryWordlist: "words.txt",
		IsFuzz:          true,
	})

	scanLines := strings.Split(scanBuf.String(), "\n")
	fuzzLines := strings.Split(fuzzBuf.String(), "\n")

	// Verify top and bottom separators match exactly
	if scanLines[0] != fuzzLines[0] {
		t.Errorf("Top separator mismatch between scan and fuzz:\nScan: %q\nFuzz: %q", scanLines[0], fuzzLines[0])
	}
	if !strings.Contains(scanLines[1], "SCAN CONFIGURATION") {
		t.Errorf("Expected SCAN CONFIGURATION header, got: %q", scanLines[1])
	}
	if !strings.Contains(fuzzLines[1], "FUZZ CONFIGURATION") {
		t.Errorf("Expected FUZZ CONFIGURATION header, got: %q", fuzzLines[1])
	}

	// Verify dot padding style matches on Target row
	if !strings.Contains(scanLines[3], "Target ....................") {
		t.Errorf("Scan target dot alignment mismatch: %q", scanLines[3])
	}
	if !strings.Contains(fuzzLines[3], "Target ....................") {
		t.Errorf("Fuzz target dot alignment mismatch: %q", fuzzLines[3])
	}
}
