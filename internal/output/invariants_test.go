package output_test

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
)

func sampleResults() []engine.Result {
	return []engine.Result{
		{
			URL:         "http://example.com/admin",
			StatusCode:  200,
			Length:      1536,
			Title:       "Admin Dashboard",
			RedirectURL: "",
			Accepted:    true,
		},
		{
			URL:         "http://example.com/login",
			StatusCode:  302,
			Length:      204800, // 200 KB
			Title:       "Redirecting...",
			RedirectURL: "http://example.com/auth/login",
			Accepted:    true,
		},
		{
			URL:         "http://example.com/api/v1/\"quotes\"",
			StatusCode:  404,
			Length:      0,
			Title:       "",
			RedirectURL: "",
			Accepted:    false,
		},
	}
}

func TestFormat_ExtensionDerivation_Table(t *testing.T) {
	tests := []struct {
		path string
		want output.Format
	}{
		{"results.json", output.FormatJSON},
		{"path/to/results.ndjson", output.FormatNDJSON},
		{"scan.csv", output.FormatCSV},
		{"report.md", output.FormatMarkdown},
		{"report.markdown", output.FormatMarkdown},
		{"output.txt", output.FormatText},
		{"output.text", output.FormatText},
		{"no_ext", output.FormatText},
		{"unknown.xyz", output.FormatText},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := output.FormatFromPath(tc.path)
			if got != tc.want {
				t.Errorf("FormatFromPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestJSONFormatter_Invariants(t *testing.T) {
	var buf bytes.Buffer
	f := output.New(output.FormatJSON, &buf, false, true, true, false)

	for _, res := range sampleResults() {
		if err := f.Print(res); err != nil {
			t.Fatalf("Print failed: %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	var parsed []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON output is not a valid JSON array: %v\nOutput:\n%s", err, buf.String())
	}

	if len(parsed) != len(sampleResults()) {
		t.Errorf("expected %d JSON objects, got %d", len(sampleResults()), len(parsed))
	}
}

func TestNDJSONFormatter_Invariants(t *testing.T) {
	var buf bytes.Buffer
	f := output.New(output.FormatNDJSON, &buf, false, true, true, false)

	results := sampleResults()
	for _, res := range results {
		if err := f.Print(res); err != nil {
			t.Fatalf("Print failed: %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != len(results) {
		t.Fatalf("expected %d lines in NDJSON output, got %d", len(results), len(lines))
	}

	for i, line := range lines {
		var item map[string]interface{}
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestCSVFormatter_Invariants(t *testing.T) {
	var buf bytes.Buffer
	f := output.New(output.FormatCSV, &buf, false, true, true, false)

	results := sampleResults()
	for _, res := range results {
		if err := f.Print(res); err != nil {
			t.Fatalf("Print failed: %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV output: %v", err)
	}

	// 1 header row + len(results) data rows
	expectedRows := 1 + len(results)
	if len(records) != expectedRows {
		t.Errorf("expected %d CSV rows, got %d", expectedRows, len(records))
	}
}

func TestMarkdownFormatter_Invariants(t *testing.T) {
	var buf bytes.Buffer
	f := output.New(output.FormatMarkdown, &buf, false, true, true, false)

	results := sampleResults()
	for _, res := range results {
		if err := f.Print(res); err != nil {
			t.Fatalf("Print failed: %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "| URL |") || !strings.Contains(out, "| Status |") {
		t.Errorf("expected Markdown table header, got:\n%s", out)
	}
	if !strings.Contains(out, "| --- |") {
		t.Errorf("expected Markdown table divider, got:\n%s", out)
	}
	for _, res := range results {
		if !strings.Contains(out, res.URL) {
			t.Errorf("expected markdown output to contain %q, got:\n%s", res.URL, out)
		}
	}
}

func TestHumanReadableFormatting_Invariants(t *testing.T) {
	var buf bytes.Buffer
	f := output.New(output.FormatText, &buf, false, false, false, true)

	res := engine.Result{
		URL:        "http://example.com/huge",
		StatusCode: 200,
		Length:     1048576, // 1 MB
		Accepted:   true,
	}

	if err := f.Print(res); err != nil {
		t.Fatalf("Print failed: %v", err)
	}
	_ = f.Close()

	out := buf.String()
	if !strings.Contains(out, "1.00 MB") && !strings.Contains(out, "1 MB") && !strings.Contains(out, "1.0 MB") {
		t.Errorf("expected human-readable size in output, got:\n%s", out)
	}
}
