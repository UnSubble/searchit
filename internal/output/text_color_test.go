package output

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/color"
	"github.com/unsubble/searchit/internal/engine"
)

func TestTextFormatter_Colors(t *testing.T) {
	res200 := engine.Result{
		StatusCode: 200,
		URL:        "http://example.com/ok",
		Length:     42,
	}
	res301 := engine.Result{
		StatusCode:  301,
		URL:         "http://example.com/redir",
		RedirectURL: "http://example.com/new",
		Length:      0,
		Headers:     http.Header{"Location": []string{"http://example.com/new"}},
	}
	res404 := engine.Result{
		StatusCode: 404,
		URL:        "http://example.com/notfound",
		Length:     10,
	}
	res500 := engine.Result{
		StatusCode: 500,
		URL:        "http://example.com/error",
		Length:     100,
	}

	t.Run("colors disabled produce no ANSI", func(t *testing.T) {
		var buf bytes.Buffer
		f := NewColoredTextFormatter(&buf, false, false, false, false, false)
		_ = f.Print(res200)
		_ = f.Print(res301)
		_ = f.Print(res404)
		_ = f.Print(res500)

		out := buf.String()
		if color.HasANSI(out) {
			t.Errorf("expected no ANSI codes when colors are disabled, got:\n%s", out)
		}
		if !strings.Contains(out, "[+] 200 - 42 B - http://example.com/ok") {
			t.Errorf("expected plain 200 result, got:\n%s", out)
		}
		if !strings.Contains(out, "[301] - 0 B - /redir -> http://example.com/new") {
			t.Errorf("expected plain redirect result, got:\n%s", out)
		}
	})

	t.Run("colors enabled produce expected ANSI codes", func(t *testing.T) {
		var buf bytes.Buffer
		f := NewColoredTextFormatter(&buf, false, false, false, false, true)
		_ = f.Print(res200)

		out := buf.String()
		if !color.HasANSI(out) {
			t.Errorf("expected ANSI codes when colors are enabled, got:\n%s", out)
		}

		// [+] should be bold green: \033[1;32m[+]\033[0m
		if !strings.Contains(out, "\033[1;32m[+]\033[0m") {
			t.Errorf("expected bold green [+] in output, got:\n%s", out)
		}

		// 200 should be bold green: \033[1;32m200\033[0m
		if !strings.Contains(out, "\033[1;32m200\033[0m") {
			t.Errorf("expected bold green 200 in output, got:\n%s", out)
		}

		// Byte received label should be bold white: \033[1;37m42 B\033[0m
		if !strings.Contains(out, "\033[1;37m42 B\033[0m -") {
			t.Errorf("expected bold white '42 B' in output, got:\n%s", out)
		}

		// URL must NOT be bold
		if strings.Contains(out, "\033[1mhttp://example.com/ok") || strings.Contains(out, "\033[1;32mhttp://example.com/ok") {
			t.Errorf("URL should remain normal weight, got:\n%s", out)
		}

		// Stripped output should match normal plain output
		if color.Strip(out) != "[+] 200 - 42 B - http://example.com/ok\n" {
			t.Errorf("stripped output mismatch: got %q", color.Strip(out))
		}
	})

	t.Run("redirect with colors enabled", func(t *testing.T) {
		var buf bytes.Buffer
		f := NewColoredTextFormatter(&buf, false, false, false, false, true)
		_ = f.Print(res301)

		out := buf.String()
		// [301] should be bold yellow: \033[1;33m[301]\033[0m (brackets and status together)
		if !strings.Contains(out, "\033[1;33m[301]\033[0m") {
			t.Errorf("expected bold yellow [301] in redirect output, got:\n%s", out)
		}
		// Byte received should be bold white: \033[1;37m0 B\033[0m
		if !strings.Contains(out, "\033[1;37m0 B\033[0m -") {
			t.Errorf("expected bold white '0 B' in redirect output, got:\n%s", out)
		}
		// Arrow should be bold white: \033[1;37m->\033[0m
		if !strings.Contains(out, "\033[1;37m->\033[0m") {
			t.Errorf("expected bold white arrow -> in redirect output, got:\n%s", out)
		}
		if color.Strip(out) != "[301] - 0 B - /redir -> http://example.com/new\n" {
			t.Errorf("stripped output mismatch: got %q", color.Strip(out))
		}
	})

	t.Run("quiet mode with colors disabled", func(t *testing.T) {
		var buf bytes.Buffer
		f := NewColoredTextFormatter(&buf, true, false, false, false, false)
		_ = f.Print(res200)

		out := buf.String()
		if color.HasANSI(out) {
			t.Errorf("expected no ANSI codes in quiet mode with colors disabled, got: %q", out)
		}
		if out != "http://example.com/ok\n" {
			t.Errorf("expected plain URL in quiet mode, got %q", out)
		}
	})

	t.Run("quiet mode with colors enabled", func(t *testing.T) {
		var buf bytes.Buffer
		f := NewColoredTextFormatter(&buf, true, false, false, false, true)
		_ = f.Print(res200)

		out := buf.String()
		if !color.HasANSI(out) {
			t.Errorf("expected ANSI codes in quiet mode with colors enabled, got: %q", out)
		}
		if out != "\033[32mhttp://example.com/ok\033[0m\n" {
			t.Errorf("expected green finding URL in quiet mode with colors enabled, got %q", out)
		}
	})

	t.Run("verbose mode with colors enabled bolds headings", func(t *testing.T) {
		var buf bytes.Buffer
		f := NewColoredTextFormatter(&buf, false, true, true, false, true)
		resVerbose := engine.Result{
			StatusCode: 200,
			URL:        "http://example.com/verbose",
			Length:     123,
			Title:      "My Page",
			Headers:    http.Header{"Server": []string{"nginx"}},
		}
		_ = f.Print(resVerbose)

		out := buf.String()
		if !strings.Contains(out, "\033[1mTITLE:\033[0m") {
			t.Errorf("expected bold TITLE: heading, got:\n%s", out)
		}
		if !strings.Contains(out, "\033[1mHEADERS:\033[0m") {
			t.Errorf("expected bold HEADERS: heading, got:\n%s", out)
		}
		if !strings.Contains(out, "\033[1;32m200\033[0m") {
			t.Errorf("expected bold green status code, got:\n%s", out)
		}
		// Content itself should not be bolded
		if strings.Contains(out, "\033[1mMy Page") || strings.Contains(out, "\033[1mnginx") {
			t.Errorf("content values must not be bolded, got:\n%s", out)
		}
	})

	t.Run("fuzz mode with non-URL fields bolds headings", func(t *testing.T) {
		var buf bytes.Buffer
		f := NewColoredTextFormatter(&buf, false, false, false, false, true)
		resFuzz := engine.Result{
			StatusCode: 200,
			URL:        "http://example.com/fuzz",
			Length:     456,
			IsFuzz:     true,
			FuzzData: &engine.FuzzData{
				Fields: []engine.FuzzField{
					{Location: engine.LocationHeader, Name: "X-Test", Value: "123"},
				},
			},
		}
		_ = f.Print(resFuzz)

		out := buf.String()
		if !strings.Contains(out, "\033[1;32m[+]\033[0m") {
			t.Errorf("expected bold green [+], got:\n%s", out)
		}
		if !strings.Contains(out, "\033[1mURL\033[0m") {
			t.Errorf("expected bold URL heading, got:\n%s", out)
		}
		if !strings.Contains(out, "\033[1mHeader\033[0m") {
			t.Errorf("expected bold Header heading, got:\n%s", out)
		}
	})

	t.Run("SetColor dynamically updates state", func(t *testing.T) {
		var buf bytes.Buffer
		f := NewColoredTextFormatter(&buf, false, false, false, false, false)
		_ = f.Print(res200)
		if color.HasANSI(buf.String()) {
			t.Errorf("expected no ANSI initially")
		}

		buf.Reset()
		f.SetColor(true)
		_ = f.Print(res200)
		if !color.HasANSI(buf.String()) {
			t.Errorf("expected ANSI after SetColor(true)")
		}

		buf.Reset()
		f.SetColor(false)
		_ = f.Print(res200)
		if color.HasANSI(buf.String()) {
			t.Errorf("expected no ANSI after SetColor(false)")
		}
	})
}

func TestFormatSizeStyled(t *testing.T) {
	palette := color.New(true)
	plainPalette := color.New(false)

	tests := []struct {
		name          string
		length        int64
		humanReadable bool
		wantStyled    string
		wantPlain     string
	}{
		{"negative length raw", -1, false, "\033[1;37m? B\033[0m", "? B"},
		{"negative length human", -1, true, "\033[1;37m? B\033[0m", "? B"},
		{"zero bytes raw", 0, false, "\033[1;37m0 B\033[0m", "0 B"},
		{"zero bytes human", 0, true, "\033[1;37m0 B\033[0m", "0 B"},
		{"small bytes raw", 42, false, "\033[1;37m42 B\033[0m", "42 B"},
		{"small bytes human", 42, true, "\033[1;37m42 B\033[0m", "42 B"},
		{"kilobytes raw", 2048, false, "\033[1;37m2048 B\033[0m", "2048 B"},
		{"kilobytes human", 2048, true, "\033[1;37m2.0 KB\033[0m", "2.0 KB"},
		{"megabytes raw", 2097152, false, "\033[1;37m2097152 B\033[0m", "2097152 B"},
		{"megabytes human", 2097152, true, "\033[1;37m2.0 MB\033[0m", "2.0 MB"},
		{"gigabytes human", 3221225472, true, "\033[1;37m3.0 GB\033[0m", "3.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStyled := FormatSizeStyled(tt.length, tt.humanReadable, palette)
			if gotStyled != tt.wantStyled {
				t.Errorf("FormatSizeStyled(%d, %v, enabled) = %q; want %q", tt.length, tt.humanReadable, gotStyled, tt.wantStyled)
			}
			if color.Strip(gotStyled) != tt.wantPlain {
				t.Errorf("Strip(gotStyled) = %q; want %q", color.Strip(gotStyled), tt.wantPlain)
			}

			// With disabled palette
			gotDisabled := FormatSizeStyled(tt.length, tt.humanReadable, plainPalette)
			if gotDisabled != tt.wantPlain {
				t.Errorf("FormatSizeStyled(%d, %v, disabled) = %q; want %q", tt.length, tt.humanReadable, gotDisabled, tt.wantPlain)
			}

			// With nil palette
			gotNil := FormatSizeStyled(tt.length, tt.humanReadable, nil)
			if gotNil != tt.wantPlain {
				t.Errorf("FormatSizeStyled(%d, %v, nil) = %q; want %q", tt.length, tt.humanReadable, gotNil, tt.wantPlain)
			}

			// Calling legacy FormatSize
			gotLegacy := FormatSize(tt.length, tt.humanReadable)
			if gotLegacy != tt.wantPlain {
				t.Errorf("FormatSize(%d, %v) = %q; want %q", tt.length, tt.humanReadable, gotLegacy, tt.wantPlain)
			}
		})
	}
}
