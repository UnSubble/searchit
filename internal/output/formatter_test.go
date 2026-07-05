package output_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
)

func TestTextFormatter(t *testing.T) {
	t.Run("identical output formatting", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewTextFormatter(&buf, false)

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
		want := "[+] 200 - http://example.com/admin\n"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("empty output", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewTextFormatter(&buf, false)
		if err := f.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
		if got := buf.String(); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("quiet mode prints only URL", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewTextFormatter(&buf, true)

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
		f := output.NewJSONFormatter(&buf)
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
		f := output.NewJSONFormatter(&buf)

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
		f := output.NewNDJSONFormatter(&buf)
		_ = f.Close()
		if got := buf.String(); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("streaming and one object per line", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewNDJSONFormatter(&buf)

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
	f := output.NewNDJSONFormatter(&buf)
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
		f := output.NewJSONFormatter(&buf)
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
		f := output.NewTextFormatter(errorWriter{}, false)
		if err := f.Print(r); err == nil {
			t.Error("expected print error, got nil")
		}
	})

	t.Run("TextFormatter quiet write error", func(t *testing.T) {
		f := output.NewTextFormatter(errorWriter{}, true)
		if err := f.Print(r); err == nil {
			t.Error("expected print error, got nil")
		}
	})

	t.Run("NDJSONFormatter write error", func(t *testing.T) {
		f := output.NewNDJSONFormatter(errorWriter{})
		if err := f.Print(r); err == nil {
			t.Error("expected print error, got nil")
		}
	})

	t.Run("JSONFormatter close write error", func(t *testing.T) {
		f := output.NewJSONFormatter(errorWriter{})
		_ = f.Print(r)
		if err := f.Close(); err == nil {
			t.Error("expected close error, got nil")
		}
	})

	t.Run("JSONFormatter close empty write error", func(t *testing.T) {
		f := output.NewJSONFormatter(errorWriter{})
		if err := f.Close(); err == nil {
			t.Error("expected close error, got nil")
		}
	})
}

