package terminal_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	terminal "github.com/unsubble/searchit/internal/output/terminal"
)

// ── Width & ContentWidth ────────────────────────────────────────────────────

func TestWidth_NonTerminalWriter(t *testing.T) {
	// bytes.Buffer is never a terminal — must return defaultWidth (80).
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	got := tm.Width()
	if got != 80 {
		t.Errorf("Width(non-terminal) = %d, want 80", got)
	}
}

func TestContentWidth_NonTerminalWriter(t *testing.T) {
	// ContentWidth clamps Width(w)-8 to [72, 96].
	// Width(buf) = 80, so 80-8 = 72 → clamped to 72.
	var buf bytes.Buffer
	tm := terminal.New(&buf)
	got := tm.ContentWidth()
	if got != 72 {
		t.Errorf("ContentWidth(80-col) = %d, want 72", got)
	}
}

// ── Separator ─────────────────────────────────────────────────────────────────

func TestSeparator_ExplicitWidth(t *testing.T) {
	got := terminal.Separator(10, "")
	want := "----------" // default char is '-'
	if got != want {
		t.Errorf("Separator(10, \"\") = %q, want %q", got, want)
	}
}

func TestSeparator_CustomChar(t *testing.T) {
	got := terminal.Separator(5, "─")
	want := "─────"
	if got != want {
		t.Errorf("Separator(5, \"─\") = %q, want %q", got, want)
	}
}

func TestSeparator_ZeroWidth_UsesTerminalWidth(t *testing.T) {
	// 0 falls back to defaultWidth (80)
	got := terminal.Separator(0, "-")
	want := strings.Repeat("-", 80)
	if got != want {
		t.Errorf("Separator(0, \"-\") = %q, want %q", got, want)
	}
}

// ── CenterTitle ───────────────────────────────────────────────────────────────

func TestCenterTitle_Short(t *testing.T) {
	// "HELLO" (5 chars) centered in 10: padding = (10-5)/2 = 2 → "  HELLO"
	got := terminal.CenterTitle("HELLO", 10)
	if got != "  HELLO" {
		t.Errorf("CenterTitle(\"HELLO\", 10) = %q, want %q", got, "  HELLO")
	}
}

func TestCenterTitle_TooLong(t *testing.T) {
	long := strings.Repeat("X", 20)
	got := terminal.CenterTitle(long, 10)
	if got != long {
		t.Errorf("CenterTitle(long, 10): expected identity, got %q", got)
	}
}

func TestCenterTitle_ExactFit(t *testing.T) {
	got := terminal.CenterTitle("HELLO", 5)
	if got != "HELLO" {
		t.Errorf("CenterTitle exact fit = %q, want %q", got, "HELLO")
	}
}

// ── RenderBlock ───────────────────────────────────────────────────────────────

func TestRenderBlock_BasicStructure(t *testing.T) {
	var buf bytes.Buffer
	items := []terminal.Item{
		{Key: "Candidates", Value: "5000"},
		{Key: "Findings", Value: "42"},
	}

	terminal.RenderBlock(&buf, "SCAN SUMMARY", items, 72)

	out := buf.String()
	if !strings.Contains(out, "SCAN SUMMARY") {
		t.Error("RenderBlock must contain title")
	}
	if !strings.Contains(out, "Candidates") {
		t.Error("RenderBlock must contain key")
	}
	if !strings.Contains(out, "5000") {
		t.Error("RenderBlock must contain value")
	}
	// Must have at least two separator lines.
	count := strings.Count(out, "---")
	if count < 2 {
		t.Errorf("RenderBlock expected ≥2 separator segments, got %d", count)
	}
}

func TestRenderBlock_EmptyTitle(t *testing.T) {
	var buf bytes.Buffer
	terminal.RenderBlock(&buf, "", []terminal.Item{{Key: "K", Value: "V"}}, 72)
	got := buf.String()
	if !strings.Contains(got, "K                            V") {
		t.Errorf("RenderBlock missing padded item without title")
	}
}

func TestRenderBlock_EmptyItems(t *testing.T) {
	var buf bytes.Buffer
	terminal.RenderBlock(&buf, "EMPTY", nil, 72)
	out := buf.String()
	if !strings.Contains(out, "EMPTY") {
		t.Error("RenderBlock with no items must still print title")
	}
}

// ── FormatLatency ─────────────────────────────────────────────────────────────

func TestFormatLatency_Zero(t *testing.T) {
	got := terminal.FormatLatency(0)
	if got != "-" {
		t.Errorf("FormatLatency(0) = %q, want \"-\"", got)
	}
}

func TestFormatLatency_Microseconds(t *testing.T) {
	got := terminal.FormatLatency(500 * time.Microsecond)
	if got != "500µs" {
		t.Errorf("FormatLatency(500µs) = %q, want %q", got, "500µs")
	}
}

func TestFormatLatency_Milliseconds(t *testing.T) {
	got := terminal.FormatLatency(250 * time.Millisecond)
	if got != "250ms" {
		t.Errorf("FormatLatency(250ms) = %q, want %q", got, "250ms")
	}
}

func TestFormatLatency_Seconds(t *testing.T) {
	got := terminal.FormatLatency(2500 * time.Millisecond)
	if got != "2.50s" {
		t.Errorf("FormatLatency(2.5s) = %q, want %q", got, "2.50s")
	}
}
