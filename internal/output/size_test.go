package output_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name          string
		length        int64
		humanReadable bool
		want          string
	}{
		// Unknown length (-1 or negative)
		{name: "unknown negative -1 (raw)", length: -1, humanReadable: false, want: "? B"},
		{name: "unknown negative -1 (human-readable)", length: -1, humanReadable: true, want: "? B"},
		{name: "unknown negative -100 (raw)", length: -100, humanReadable: false, want: "? B"},
		{name: "unknown negative -100 (human-readable)", length: -100, humanReadable: true, want: "? B"},

		// Zero bytes
		{name: "zero bytes (raw)", length: 0, humanReadable: false, want: "0 B"},
		{name: "zero bytes (human-readable)", length: 0, humanReadable: true, want: "0 B"},

		// Small bytes (< 1024)
		{name: "small bytes 50 (raw)", length: 50, humanReadable: false, want: "50 B"},
		{name: "small bytes 50 (human-readable)", length: 50, humanReadable: true, want: "50 B"},
		{name: "small bytes 1023 (raw)", length: 1023, humanReadable: false, want: "1023 B"},
		{name: "small bytes 1023 (human-readable)", length: 1023, humanReadable: true, want: "1023 B"},

		// Kilobytes (>= 1024, < 1MB)
		{name: "1024 bytes (raw)", length: 1024, humanReadable: false, want: "1024 B"},
		{name: "1024 bytes (human-readable)", length: 1024, humanReadable: true, want: "1.0 KB"},
		{name: "9797 bytes (raw)", length: 9797, humanReadable: false, want: "9797 B"},
		{name: "9797 bytes (human-readable)", length: 9797, humanReadable: true, want: "9.6 KB"},

		// Megabytes (>= 1MB, < 1GB)
		{name: "1048576 bytes (raw)", length: 1048576, humanReadable: false, want: "1048576 B"},
		{name: "1048576 bytes (human-readable)", length: 1048576, humanReadable: true, want: "1.0 MB"},
		{name: "12582912 bytes (raw)", length: 12582912, humanReadable: false, want: "12582912 B"},
		{name: "12582912 bytes (human-readable)", length: 12582912, humanReadable: true, want: "12.0 MB"},

		// Gigabytes (>= 1GB)
		{name: "1073741824 bytes (raw)", length: 1073741824, humanReadable: false, want: "1073741824 B"},
		{name: "1073741824 bytes (human-readable)", length: 1073741824, humanReadable: true, want: "1.0 GB"},
		{name: "5368709120 bytes (raw)", length: 5368709120, humanReadable: false, want: "5368709120 B"},
		{name: "5368709120 bytes (human-readable)", length: 5368709120, humanReadable: true, want: "5.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := output.FormatSize(tt.length, tt.humanReadable)
			if got != tt.want {
				t.Errorf("FormatSize(%d, %v) = %q, want %q", tt.length, tt.humanReadable, got, tt.want)
			}
		})
	}
}

func TestTextFormatter_SizeFormatting(t *testing.T) {
	rNormal := engine.Result{
		URL:        "http://example.com/file.zip",
		StatusCode: 200,
		Length:     1048576,
	}

	rRedirect := engine.Result{
		URL:         "http://example.com/redir",
		StatusCode:  301,
		Length:      9797,
		RedirectURL: "http://example.com/dest",
		Headers:     http.Header{"Location": []string{"http://example.com/dest"}},
	}

	rUnknown := engine.Result{
		URL:        "http://example.com/stream",
		StatusCode: 200,
		Length:     -1,
	}

	rZero := engine.Result{
		URL:        "http://example.com/empty",
		StatusCode: 200,
		Length:     0,
	}

	t.Run("Default Raw Bytes Rendering", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewTextFormatter(&buf, false, false, false, false)

		_ = f.Print(rNormal)
		_ = f.Print(rRedirect)
		_ = f.Print(rUnknown)
		_ = f.Print(rZero)

		out := buf.String()
		if !strings.Contains(out, "[+] 200 - 1048576 B - http://example.com/file.zip") {
			t.Errorf("expected raw bytes for normal result, got:\n%s", out)
		}
		if !strings.Contains(out, "[301] - 9797 B - /redir -> http://example.com/dest") {
			t.Errorf("expected raw bytes for redirect result, got:\n%s", out)
		}
		if !strings.Contains(out, "[+] 200 - ? B - http://example.com/stream") {
			t.Errorf("expected '? B' for unknown length, got:\n%s", out)
		}
		if !strings.Contains(out, "[+] 200 - 0 B - http://example.com/empty") {
			t.Errorf("expected '0 B' for zero length, got:\n%s", out)
		}
	})

	t.Run("Human Readable Rendering", func(t *testing.T) {
		var buf bytes.Buffer
		f := output.NewTextFormatter(&buf, false, false, false, true)

		_ = f.Print(rNormal)
		_ = f.Print(rRedirect)
		_ = f.Print(rUnknown)
		_ = f.Print(rZero)

		out := buf.String()
		if !strings.Contains(out, "[+] 200 - 1.0 MB - http://example.com/file.zip") {
			t.Errorf("expected '1.0 MB' for normal result, got:\n%s", out)
		}
		if !strings.Contains(out, "[301] - 9.6 KB - /redir -> http://example.com/dest") {
			t.Errorf("expected '9.6 KB' for redirect result, got:\n%s", out)
		}
		if !strings.Contains(out, "[+] 200 - ? B - http://example.com/stream") {
			t.Errorf("expected '? B' for unknown length, got:\n%s", out)
		}
		if !strings.Contains(out, "[+] 200 - 0 B - http://example.com/empty") {
			t.Errorf("expected '0 B' for zero length, got:\n%s", out)
		}
	})

	t.Run("Verbose Text Formatter Respects Human Readable", func(t *testing.T) {
		var bufRaw bytes.Buffer
		fRaw := output.NewTextFormatter(&bufRaw, false, true, true, false)
		_ = fRaw.Print(rNormal)
		if !strings.Contains(bufRaw.String(), "200     1048576 B\n\nhttp://example.com/file.zip") {
			t.Errorf("expected raw bytes in verbose output, got:\n%s", bufRaw.String())
		}

		var bufHR bytes.Buffer
		fHR := output.NewTextFormatter(&bufHR, false, true, true, true)
		_ = fHR.Print(rNormal)
		if !strings.Contains(bufHR.String(), "200     1.0 MB\n\nhttp://example.com/file.zip") {
			t.Errorf("expected human readable in verbose output, got:\n%s", bufHR.String())
		}
	})
}
