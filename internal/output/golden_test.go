package output_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
)

var goldenResults = []engine.Result{
	{URL: "http://example.com/admin", StatusCode: 200, Length: 1234, Depth: 0},
	{URL: "http://example.com/login", StatusCode: 403, Length: 56, Depth: 1},
	{URL: "http://example.com/api/v1", StatusCode: 302, Length: 0, Depth: 2},
}

func TestGoldenOutputs(t *testing.T) {
	tests := []struct {
		name       string
		goldenFile string
		setup      func(buf *bytes.Buffer) output.Formatter
	}{
		{
			name:       "Text formatter",
			goldenFile: "text.golden",
			setup: func(buf *bytes.Buffer) output.Formatter {
				return output.NewTextFormatter(buf, false)
			},
		},
		{
			name:       "Quiet Text formatter",
			goldenFile: "text_quiet.golden",
			setup: func(buf *bytes.Buffer) output.Formatter {
				return output.NewTextFormatter(buf, true)
			},
		},
		{
			name:       "JSON formatter",
			goldenFile: "json.golden",
			setup: func(buf *bytes.Buffer) output.Formatter {
				return output.NewJSONFormatter(buf)
			},
		},
		{
			name:       "NDJSON formatter",
			goldenFile: "ndjson.golden",
			setup: func(buf *bytes.Buffer) output.Formatter {
				return output.NewNDJSONFormatter(buf)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			fmttr := tc.setup(&buf)

			for _, r := range goldenResults {
				if err := fmttr.Print(r); err != nil {
					t.Fatalf("Print failed: %v", err)
				}
			}

			if err := fmttr.Close(); err != nil {
				t.Fatalf("Close failed: %v", err)
			}

			goldenPath := filepath.Join("testdata", tc.goldenFile)
			expected, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
			}

			got := buf.Bytes()
			if !bytes.Equal(got, expected) {
				t.Errorf("output mismatch for %s.\nGot:\n%s\nExpected:\n%s", tc.goldenFile, string(got), string(expected))
			}
		})
	}
}
