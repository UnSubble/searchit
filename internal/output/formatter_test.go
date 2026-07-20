package output_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
)

func TestTextFormatter(t *testing.T) {
	t.Run("identical output formatting", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewTextFormatter(&buf, false, false, false)

		r := engine.Result{
			URL:        "http://example.com/admin",
			StatusCode: 200,
			Length:     -1,
		}

		if err := f.Print(r); err != nil {
			t.Fatalf("Print failed: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		got := buf.String()
		want := "[+] 200 -        - http://example.com/admin\n"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("empty output", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewTextFormatter(&buf, false, false, false)
		if err := f.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
		if got := buf.String(); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("quiet mode prints only URL", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewTextFormatter(&buf, true, false, false)

		r := engine.Result{
			URL:        "http://example.com/admin",
			StatusCode: 200,
		}

		if err := f.Print(r); err != nil {
			t.Fatalf("Print failed: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		got := buf.String()
		want := "http://example.com/admin\n"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestJSONFormatter(t *testing.T) {
	t.Run("empty array", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewJSONFormatter(&buf, false, false)
		if err := f.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
		got := buf.String()
		want := "[]\n"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("multiple results pretty printed", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewJSONFormatter(&buf, false, false)

		r1 := engine.Result{URL: "http://example.com/a", StatusCode: 200, Length: 10, Depth: 1}
		r2 := engine.Result{URL: "http://example.com/b", StatusCode: 403, Length: 0, Depth: 2}

		_ = f.Print(r1)
		_ = f.Print(r2)
		_ = f.Close()

		got := buf.String()
		var parsed []map[string]any
		if err := json.Unmarshal([]byte(got), &parsed); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v. Output was:\n%s", err, got)
		}

		if len(parsed) != 2 {
			t.Fatalf("got %d elements, want 2", len(parsed))
		}

		if parsed[0]["url"] != "http://example.com/a" || parsed[0]["status"] != 200.0 || parsed[0]["length"] != 10.0 || parsed[0]["depth"] != 1.0 {
			t.Errorf("r1 fields mismatch: %+v", parsed[0])
		}

		if !strings.Contains(got, "\n  {\n") {
			t.Errorf("expected pretty printing with two spaces, got:\n%s", got)
		}
	})
}

func TestNDJSONFormatter(t *testing.T) {
	t.Run("empty output", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewNDJSONFormatter(&buf, false, false)
		_ = f.Close()
		if got := buf.String(); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("streaming and one object per line", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewNDJSONFormatter(&buf, false, false)

		r1 := engine.Result{URL: "http://example.com/a", StatusCode: 200, Length: 10, Depth: 1}
		if err := f.Print(r1); err != nil {
			t.Fatalf("Print failed: %v", err)
		}

		gotStreaming := buf.String()
		if gotStreaming == "" {
			t.Error("expected immediate write (streaming), but got empty buffer before Close")
		}

		r2 := engine.Result{URL: "http://example.com/b", StatusCode: 403, Length: 0, Depth: 2}
		_ = f.Print(r2)
		_ = f.Close()

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("got %d lines, want 2", len(lines))
		}

		for i, line := range lines {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(line), &parsed); err != nil {
				t.Fatalf("line %d is invalid JSON: %v", i+1, err)
			}
		}
	})
}

func BenchmarkFormatterNDJSON(b *testing.B) {
	var buf bytes.Buffer
	f := output.NewNDJSONFormatter(&buf, false, false)
	r := engine.Result{
		URL:        "http://example.com/admin/login/dashboard/v2/index.html",
		StatusCode: 200,
		Length:     1024,
		Depth:      3,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = f.Print(r)
	}
}

func BenchmarkFormatterJSON(b *testing.B) {
	var buf bytes.Buffer
	r := engine.Result{
		URL:        "http://example.com/admin/login/dashboard/v2/index.html",
		StatusCode: 200,
		Length:     1024,
		Depth:      3,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		f := output.NewJSONFormatter(&buf, false, false)
		_ = f.Print(r)
		_ = f.Print(r)
		_ = f.Close()
	}
}

type errorWriter struct{}

func (errorWriter) Write(p []byte) (int, error) {
	return 0, os.ErrPermission // any error
}

func TestFormatterErrors(t *testing.T) {
	r := engine.Result{URL: "http://example.com", StatusCode: 200}

	t.Run("TextFormatter write error", func(t *testing.T) {
		f := output.NewTextFormatter(errorWriter{}, false, false, false)
		if err := f.Print(r); err == nil {
			t.Error("expected print error, got nil")
		}
	})

	t.Run("TextFormatter quiet write error", func(t *testing.T) {
		f := output.NewTextFormatter(errorWriter{}, true, false, false)
		if err := f.Print(r); err == nil {
			t.Error("expected print error, got nil")
		}
	})

	t.Run("NDJSONFormatter write error", func(t *testing.T) {
		f := output.NewNDJSONFormatter(errorWriter{}, false, false)
		if err := f.Print(r); err == nil {
			t.Error("expected print error, got nil")
		}
	})

	t.Run("JSONFormatter close write error", func(t *testing.T) {
		f := output.NewJSONFormatter(errorWriter{}, false, false)
		_ = f.Print(r)
		if err := f.Close(); err == nil {
			t.Error("expected close error, got nil")
		}
	})

	t.Run("JSONFormatter close empty write error", func(t *testing.T) {
		f := output.NewJSONFormatter(errorWriter{}, false, false)
		if err := f.Close(); err == nil {
			t.Error("expected close error, got nil")
		}
	})
}

func TestCSVFormatter(t *testing.T) {
	r1 := engine.Result{URL: "http://example.com/a", StatusCode: 200, Length: 10, Depth: 1}
	r2 := engine.Result{URL: "http://example.com/b", StatusCode: 403, Length: 0, Depth: 2}

	t.Run("no output when no Print before close", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewCSVFormatter(&buf, false, false)
		_ = f.Close()
		if got := buf.String(); got != "" {
			t.Errorf("expected empty output, got %q", got)
		}
	})

	t.Run("header written on first Print", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewCSVFormatter(&buf, false, false)
		_ = f.Print(r1)
		_ = f.Close()
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines (header + 1 row), got %d: %q", len(lines), buf.String())
		}
		if lines[0] != "url,status,length,depth" {
			t.Errorf("header = %q, want %q", lines[0], "url,status,length,depth")
		}
	})

	t.Run("multiple results all present", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewCSVFormatter(&buf, false, false)
		_ = f.Print(r1)
		_ = f.Print(r2)
		_ = f.Close()
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		if len(lines) != 3 {
			t.Fatalf("expected 3 lines, got %d", len(lines))
		}
		if lines[1] != "http://example.com/a,200,10,1" {
			t.Errorf("row 1 = %q", lines[1])
		}
		if lines[2] != "http://example.com/b,403,0,2" {
			t.Errorf("row 2 = %q", lines[2])
		}
	})

	t.Run("URL with comma is quoted", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewCSVFormatter(&buf, false, false)
		r := engine.Result{URL: "http://example.com/a,b", StatusCode: 200}
		_ = f.Print(r)
		_ = f.Close()
		if !strings.Contains(buf.String(), `"http://example.com/a,b"`) {
			t.Errorf("expected quoted URL, got %q", buf.String())
		}
	})

	t.Run("streaming: data present before Close", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewCSVFormatter(&buf, false, false)
		_ = f.Print(r1)
		if buf.String() == "" {
			t.Error("expected data before Close (streaming)")
		}
	})
}

func TestMarkdownFormatter(t *testing.T) {
	r1 := engine.Result{URL: "http://example.com/a", StatusCode: 200, Length: 10, Depth: 1}
	r2 := engine.Result{URL: "http://example.com/b", StatusCode: 403, Length: 0, Depth: 2}

	t.Run("empty output when no Print called", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewMarkdownFormatter(&buf, false, false)
		_ = f.Close()
		if got := buf.String(); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("header and separator on first Print", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewMarkdownFormatter(&buf, false, false)
		_ = f.Print(r1)
		_ = f.Close()
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
		if len(lines) != 3 {
			t.Fatalf("expected 3 lines (header+sep+row), got %d: %q", len(lines), buf.String())
		}
		if lines[0] != "| URL | Status | Length | Depth |" {
			t.Errorf("header = %q", lines[0])
		}
		if lines[1] != "| --- | -----: | -----: | ----: |" {
			t.Errorf("separator = %q", lines[1])
		}
	})

	t.Run("multiple results all present", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewMarkdownFormatter(&buf, false, false)
		_ = f.Print(r1)
		_ = f.Print(r2)
		_ = f.Close()
		if !strings.Contains(buf.String(), "http://example.com/a") {
			t.Error("expected r1 URL in output")
		}
		if !strings.Contains(buf.String(), "http://example.com/b") {
			t.Error("expected r2 URL in output")
		}
	})

	t.Run("streaming: data before Close", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewMarkdownFormatter(&buf, false, false)
		_ = f.Print(r1)
		if buf.String() == "" {
			t.Error("expected data before Close (streaming)")
		}
	})
}

func TestFormatParse(t *testing.T) {
	valid := []struct {
		in   string
		want output.Format
	}{
		{"text", output.FormatText},
		{"TEXT", output.FormatText},
		{"json", output.FormatJSON},
		{"JSON", output.FormatJSON},
		{"ndjson", output.FormatNDJSON},
		{"NDJSON", output.FormatNDJSON},
		{"csv", output.FormatCSV},
		{"CSV", output.FormatCSV},
		{"markdown", output.FormatMarkdown},
		{"MARKDOWN", output.FormatMarkdown},
	}
	for _, tc := range valid {
		f, err := output.Parse(tc.in)
		if err != nil {
			t.Errorf("Parse(%q) error = %v", tc.in, err)
			continue
		}
		if f != tc.want {
			t.Errorf("Parse(%q) = %q, want %q", tc.in, f, tc.want)
		}
	}

	invalid := []string{"", "xml", "yaml", "html", "pdf"}
	for _, s := range invalid {
		if _, err := output.Parse(s); err == nil {
			t.Errorf("Parse(%q) expected error, got nil", s)
		}
	}
}

func TestFormatFromExtension(t *testing.T) {
	tests := []struct {
		ext  string
		want output.Format
	}{
		{".txt", output.FormatText},
		{".text", output.FormatText},
		{".json", output.FormatJSON},
		{".ndjson", output.FormatNDJSON},
		{".csv", output.FormatCSV},
		{".md", output.FormatMarkdown},
		{".markdown", output.FormatMarkdown},
		{".xml", output.FormatText},
		{"", output.FormatText},
		{".log", output.FormatText},
	}
	for _, tc := range tests {
		got := output.FormatFromExtension(tc.ext)
		if got != tc.want {
			t.Errorf("FormatFromExtension(%q) = %q, want %q", tc.ext, got, tc.want)
		}
	}
}

func TestFormatFromPath(t *testing.T) {
	tests := []struct {
		path string
		want output.Format
	}{
		{"results.json", output.FormatJSON},
		{"report.ndjson", output.FormatNDJSON},
		{"findings.csv", output.FormatCSV},
		{"report.md", output.FormatMarkdown},
		{"out.txt", output.FormatText},
		{"out", output.FormatText},
		{"out.xml", output.FormatText},
		{"/tmp/scan.json", output.FormatJSON},
	}
	for _, tc := range tests {
		got := output.FormatFromPath(tc.path)
		if got != tc.want {
			t.Errorf("FormatFromPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestSupportedFormats(t *testing.T) {
	supported := output.SupportedFormats()
	for _, name := range []string{"text", "json", "ndjson", "csv", "markdown"} {
		found := false
		for _, s := range supported {
			if s == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("SupportedFormats() missing %q; got %v", name, supported)
		}
	}
}

func TestNew(t *testing.T) {
	r := engine.Result{URL: "http://example.com", StatusCode: 200, Length: 1, Depth: 0}
	for _, f := range []output.Format{output.FormatText, output.FormatJSON, output.FormatNDJSON, output.FormatCSV, output.FormatMarkdown} {
		var buf bytes.Buffer
		fmttr := output.New(f, &buf, false, false, false)
		if fmttr == nil {
			t.Errorf("New(%q) returned nil", f)
			continue
		}
		_ = fmttr.Print(r)
		if err := fmttr.Close(); err != nil {
			t.Errorf("New(%q).Close() = %v", f, err)
		}
		if buf.Len() == 0 {
			t.Errorf("New(%q) produced no output", f)
		}
	}
}

func TestFormatters_ShowPresentation(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Server", "nginx")
	headers.Set("Content-Type", "text/html")

	r := engine.Result{
		URL:        "http://example.com/admin",
		StatusCode: 200,
		Length:     512,
		Title:      "Admin Dashboard",
		Headers:    headers,
	}

	t.Run("TextFormatter with show-title and show-headers", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewTextFormatter(&buf, false, true, true)
		if err := f.Print(r); err != nil {
			t.Fatalf("Print failed: %v", err)
		}

		got := buf.String()
		want := "200     512B\n\nhttp://example.com/admin\n" +
			"\nTITLE:\n------\nAdmin Dashboard\n------\n" +
			"\nHEADERS:\n--------\nContent-Type: text/html\nServer: nginx\n--------\n"
		if got != want {
			t.Errorf("got:\n%q\nwant:\n%q", got, want)
		}
	})

	t.Run("JSONFormatter with show-title and show-headers", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewJSONFormatter(&buf, true, true)
		_ = f.Print(r)
		_ = f.Close()

		var parsed []map[string]any
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatalf("JSON parse error: %v", err)
		}
		if len(parsed) != 1 {
			t.Fatalf("expected 1 item, got %d", len(parsed))
		}
		if parsed[0]["title"] != "Admin Dashboard" {
			t.Errorf("expected title 'Admin Dashboard', got %v", parsed[0]["title"])
		}
		headersMap := parsed[0]["headers"].(map[string]any)
		if headersMap["Server"].([]any)[0] != "nginx" {
			t.Errorf("expected header Server: nginx, got %v", headersMap["Server"])
		}
	})

	t.Run("CSV and Markdown with show-headers", func(t *testing.T) {
		// Test CSV with headers
		var csvBuf bytes.Buffer
		csvFormatter := output.NewCSVFormatter(&csvBuf, true, true)
		_ = csvFormatter.Print(r)
		_ = csvFormatter.Close()
		csvStr := csvBuf.String()
		if !strings.Contains(csvStr, "Server") || !strings.Contains(csvStr, "nginx") {
			t.Errorf("expected CSV headers/values in output, got:\n%s", csvStr)
		}

		// Test Markdown with headers
		var mdBuf bytes.Buffer
		mdFormatter := output.NewMarkdownFormatter(&mdBuf, true, true)
		_ = mdFormatter.Print(r)
		_ = mdFormatter.Close()
		mdStr := mdBuf.String()
		if !strings.Contains(mdStr, "Server") || !strings.Contains(mdStr, "nginx") {
			t.Errorf("expected Markdown headers/values in output, got:\n%s", mdStr)
		}
	})
}
