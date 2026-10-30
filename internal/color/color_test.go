package color

import (
	"bytes"
	"testing"
)

func TestIsValidMode(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"auto", true},
		{"always", true},
		{"never", true},
		{"AUTO", true},
		{"Always", true},
		{"Never", true},
		{" auto ", true},
		{"invalid", false},
		{"false", false},
		{"true", false},
		{"", false},
		{"none", false},
	}

	for _, tt := range tests {
		got := IsValidMode(tt.input)
		if got != tt.want {
			t.Errorf("IsValidMode(%q) = %v; want %v", tt.input, got, tt.want)
		}
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		quiet    bool
		isTTY    bool
		expected bool
	}{
		// auto mode without quiet
		{"auto with TTY", "auto", false, true, true},
		{"auto without TTY", "auto", false, false, false},
		{"default (empty) with TTY", "", false, true, true},
		{"default (empty) without TTY", "", false, false, false},

		// auto mode with quiet
		{"auto with quiet and TTY", "auto", true, true, false},
		{"auto with quiet without TTY", "auto", true, false, false},
		{"default (empty) with quiet and TTY", "", true, true, false},

		// always mode
		{"always without quiet, with TTY", "always", false, true, true},
		{"always without quiet, without TTY", "always", false, false, true},
		{"always with quiet, with TTY", "always", true, true, true},
		{"always with quiet, without TTY", "always", true, false, true},

		// never mode
		{"never without quiet, with TTY", "never", false, true, false},
		{"never without quiet, without TTY", "never", false, false, false},
		{"never with quiet, with TTY", "never", true, true, false},
		{"never with quiet, without TTY", "never", true, false, false},

		// invalid mode
		{"invalid mode", "foo", false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.mode, tt.quiet, func() bool { return tt.isTTY })
			if got != tt.expected {
				t.Errorf("Resolve(%q, quiet=%v, tty=%v) = %v; want %v",
					tt.mode, tt.quiet, tt.isTTY, got, tt.expected)
			}
		})
	}
}

func TestPalette_Disabled(t *testing.T) {
	p := New(false)
	if p.Enabled() {
		t.Errorf("expected palette to be disabled")
	}

	if got := p.Success("ok"); got != "ok" || HasANSI(got) || HasBold(got) {
		t.Errorf("expected plain 'ok', got %q", got)
	}
	if got := p.Warning("warn"); got != "warn" || HasANSI(got) || HasBold(got) {
		t.Errorf("expected plain 'warn', got %q", got)
	}
	if got := p.Error("fail"); got != "fail" || HasANSI(got) || HasBold(got) {
		t.Errorf("expected plain 'fail', got %q", got)
	}
	if got := p.Finding("http://example.com"); got != "http://example.com" || HasANSI(got) || HasBold(got) {
		t.Errorf("expected plain 'http://example.com', got %q", got)
	}
	if got := p.Bold("heading"); got != "heading" || HasANSI(got) || HasBold(got) {
		t.Errorf("expected plain 'heading', got %q", got)
	}
	if got := p.BoldWhite("B"); got != "B" || HasANSI(got) || HasBold(got) || HasWhite(got) {
		t.Errorf("expected plain 'B', got %q", got)
	}
	if got := p.ByteReceived("B"); got != "B" || HasANSI(got) || HasBold(got) || HasWhite(got) {
		t.Errorf("expected plain 'B', got %q", got)
	}
	if got := p.ByteReceived("KB"); got != "KB" || HasANSI(got) || HasBold(got) || HasWhite(got) {
		t.Errorf("expected plain 'KB', got %q", got)
	}
	if got := p.Heading("heading"); got != "heading" || HasANSI(got) || HasBold(got) {
		t.Errorf("expected plain 'heading', got %q", got)
	}
	if got := p.Status(200); got != "200" || HasANSI(got) || HasBold(got) {
		t.Errorf("expected plain '200', got %q", got)
	}
	if got := p.Status(301); got != "301" || HasANSI(got) || HasBold(got) {
		t.Errorf("expected plain '301', got %q", got)
	}
	if got := p.Status(404); got != "404" || HasANSI(got) || HasBold(got) {
		t.Errorf("expected plain '404', got %q", got)
	}
	if got := p.Status(500); got != "500" || HasANSI(got) || HasBold(got) {
		t.Errorf("expected plain '500', got %q", got)
	}
	if got := p.StatusBracketed(301); got != "[301]" || HasANSI(got) || HasBold(got) {
		t.Errorf("expected plain '[301]', got %q", got)
	}
}

func TestPalette_Enabled(t *testing.T) {
	p := New(true)
	if !p.Enabled() {
		t.Errorf("expected palette to be enabled")
	}

	// Success: bold green
	s := p.Success("ok")
	if !HasANSI(s) || !HasBold(s) || s != "\033[1;32mok\033[0m" {
		t.Errorf("expected bold green 'ok', got %q", s)
	}
	if Strip(s) != "ok" {
		t.Errorf("Strip(%q) = %q; want 'ok'", s, Strip(s))
	}

	// Warning: bold yellow
	w := p.Warning("warn")
	if !HasANSI(w) || !HasBold(w) || w != "\033[1;33mwarn\033[0m" {
		t.Errorf("expected bold yellow 'warn', got %q", w)
	}
	if Strip(w) != "warn" {
		t.Errorf("Strip(%q) = %q; want 'warn'", w, Strip(w))
	}

	// Error: bold red
	e := p.Error("fail")
	if !HasANSI(e) || !HasBold(e) || e != "\033[1;31mfail\033[0m" {
		t.Errorf("expected bold red 'fail', got %q", e)
	}
	if Strip(e) != "fail" {
		t.Errorf("Strip(%q) = %q; want 'fail'", e, Strip(e))
	}

	// BoldWhite: bold white
	bw := p.BoldWhite("B")
	if !HasANSI(bw) || !HasBold(bw) || !HasWhite(bw) || bw != "\033[1;37mB\033[0m" {
		t.Errorf("expected bold white 'B', got %q", bw)
	}
	if Strip(bw) != "B" {
		t.Errorf("Strip(%q) = %q; want 'B'", bw, Strip(bw))
	}

	// ByteReceived: bold white label
	br := p.ByteReceived("KB")
	if !HasANSI(br) || !HasBold(br) || !HasWhite(br) || br != "\033[1;37mKB\033[0m" {
		t.Errorf("expected bold white 'KB', got %q", br)
	}
	if Strip(br) != "KB" {
		t.Errorf("Strip(%q) = %q; want 'KB'", br, Strip(br))
	}

	// Bold: bold default color
	b := p.Bold("section")
	if !HasANSI(b) || !HasBold(b) || b != "\033[1msection\033[0m" {
		t.Errorf("expected bold 'section', got %q", b)
	}
	if Strip(b) != "section" {
		t.Errorf("Strip(%q) = %q; want 'section'", b, Strip(b))
	}

	// Heading: bold default color
	h := p.Heading("URL")
	if !HasANSI(h) || !HasBold(h) || h != "\033[1mURL\033[0m" {
		t.Errorf("expected bold heading 'URL', got %q", h)
	}
	if Strip(h) != "URL" {
		t.Errorf("Strip(%q) = %q; want 'URL'", h, Strip(h))
	}

	// Finding: normal-weight green (URL must not be bold)
	f := p.Finding("http://example.com")
	if !HasANSI(f) || HasBold(f) || f != "\033[32mhttp://example.com\033[0m" {
		t.Errorf("expected normal green finding, got %q", f)
	}

	// Empty string should not be wrapped with ANSI
	if got := p.Success(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if got := p.Warning(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if got := p.Error(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if got := p.Finding(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if got := p.Bold(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if got := p.BoldWhite(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if got := p.ByteReceived(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if got := p.Heading(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}

	// Status codes: bold semantic colors
	st200 := p.Status(200)
	if st200 != "\033[1;32m200\033[0m" || !HasBold(st200) {
		t.Errorf("Status(200) = %q, want bold green 200", st200)
	}
	st301 := p.Status(301)
	if st301 != "\033[1;33m301\033[0m" || !HasBold(st301) {
		t.Errorf("Status(301) = %q, want bold yellow 301", st301)
	}
	st404 := p.Status(404)
	if st404 != "\033[1;33m404\033[0m" || !HasBold(st404) {
		t.Errorf("Status(404) = %q, want bold yellow 404", st404)
	}
	st500 := p.Status(500)
	if st500 != "\033[1;31m500\033[0m" || !HasBold(st500) {
		t.Errorf("Status(500) = %q, want bold red 500", st500)
	}
	st999 := p.Status(999)
	if st999 != "999" || HasANSI(st999) || HasBold(st999) {
		t.Errorf("Status(999) = %q, want uncolored 999", st999)
	}

	// StatusBracketed: bold semantic colors around status code and brackets
	stb301 := p.StatusBracketed(301)
	if stb301 != "\033[1;33m[301]\033[0m" || !HasBold(stb301) {
		t.Errorf("StatusBracketed(301) = %q, want bold yellow [301]", stb301)
	}
	stb200 := p.StatusBracketed(200)
	if stb200 != "\033[1;32m[200]\033[0m" || !HasBold(stb200) {
		t.Errorf("StatusBracketed(200) = %q, want bold green [200]", stb200)
	}
}

func TestTTYDetector(t *testing.T) {
	defer ResetTTYDetector()

	SetTTYDetector(func() bool { return true })
	var buf bytes.Buffer
	if !IsTerminal(&buf) {
		t.Errorf("expected IsTerminal to return true with overridden detector")
	}

	SetTTYDetector(func() bool { return false })
	if IsTerminal(&buf) {
		t.Errorf("expected IsTerminal to return false with overridden detector")
	}

	ResetTTYDetector()
	// A bytes.Buffer is not an os.File, so default IsTerminal must return false
	if IsTerminal(&buf) {
		t.Errorf("expected IsTerminal on bytes.Buffer to return false")
	}
}

func TestConfigureAndGlobal(t *testing.T) {
	defer Configure("never", false, nil)

	Configure("always", false, nil)
	if !IsEnabled() {
		t.Errorf("expected IsEnabled to be true after Configure('always')")
	}
	if !HasANSI(Success("test")) {
		t.Errorf("expected Success('test') to have ANSI codes")
	}

	Configure("never", false, nil)
	if IsEnabled() {
		t.Errorf("expected IsEnabled to be false after Configure('never')")
	}
	if HasANSI(Success("test")) {
		t.Errorf("expected Success('test') to NOT have ANSI codes")
	}
}
