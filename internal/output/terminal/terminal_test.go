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

// ── FormatDotRow ─────────────────────────────────────────────────────────────

func TestFormatDotRow_Basic(t *testing.T) {
	got := terminal.FormatDotRow("Candidates", "5000", 0, 0)
	// Default padWidth=28: "Candidates" (10 chars) → dotsCount = 28-10-2 = 16
	// Line = "Candidates ................ 5000"
	if !strings.Contains(got, "Candidates") || !strings.Contains(got, "5000") {
		t.Errorf("FormatDotRow unexpected output: %q", got)
	}
	if !strings.Contains(got, ".") {
		t.Errorf("FormatDotRow must contain dot padding, got: %q", got)
	}
}

func TestFormatDotRow_LongKey(t *testing.T) {
	// Key longer than padWidth-2 → falls through to "key .. val" form.
	key := strings.Repeat("X", 30)
	got := terminal.FormatDotRow(key, "value", 0, 0)
	if !strings.Contains(got, " .. ") {
		t.Errorf("FormatDotRow with long key should use ' .. ' separator, got: %q", got)
	}
}

func TestFormatDotRow_MaxColumns(t *testing.T) {
	got := terminal.FormatDotRow("Key", "Value", 0, 20)
	if len(got) > 20 {
		t.Errorf("FormatDotRow with maxColumns=20: len=%d, want ≤20, got: %q", len(got), got)
	}
}

func TestFormatDotRow_ZeroPadWidth_UsesDefault(t *testing.T) {
	// padWidth=0 must use DefaultPadWidth (28), not panic.
	got := terminal.FormatDotRow("A", "B", 0, 0)
	if got == "" {
		t.Error("FormatDotRow with padWidth=0 returned empty string")
	}
}

// ── FormatTwoColumnRow ───────────────────────────────────────────────────────

func TestFormatTwoColumnRow_Basic(t *testing.T) {
	got := terminal.FormatTwoColumnRow("Elapsed", "00:00:03", "ETA", "00:01:22")
	// Should start with "Elapsed     00:00:03      ETA         00:01:22"
	if !strings.HasPrefix(got, "Elapsed") {
		t.Errorf("FormatTwoColumnRow should start with left key, got: %q", got)
	}
	if !strings.Contains(got, "ETA") {
		t.Errorf("FormatTwoColumnRow should contain right key, got: %q", got)
	}
	if !strings.Contains(got, "00:01:22") {
		t.Errorf("FormatTwoColumnRow should contain right value, got: %q", got)
	}
}

func TestFormatTwoColumnRow_ColumnAlignment(t *testing.T) {
	// Two rows with the same column widths must align their right-key column
	// at the same byte index regardless of the left content.
	row1 := terminal.FormatTwoColumnRow("Requests", "100", "Responses", "99")
	row2 := terminal.FormatTwoColumnRow("Req/s", "50", "Workers", "4")

	rightStart := terminal.TwoColLeftKeyWidth + terminal.TwoColLeftValWidth

	if len(row1) < rightStart+1 {
		t.Fatalf("row1 too short: %q", row1)
	}
	if len(row2) < rightStart+1 {
		t.Fatalf("row2 too short: %q", row2)
	}

	// The byte at rightStart must NOT be a space in either row
	// (the right key starts here and must not be all-space padding).
	if row1[rightStart] == ' ' && row1[rightStart+1] == ' ' && row1[rightStart+2] == ' ' {
		t.Errorf("row1 right column appears to be empty at offset %d: %q", rightStart, row1)
	}
	if row2[rightStart] == ' ' && row2[rightStart+1] == ' ' && row2[rightStart+2] == ' ' {
		t.Errorf("row2 right column appears to be empty at offset %d: %q", rightStart, row2)
	}

	// Both rows must have the same total left-side width (bytes 0..rightStart-1).
	if row1[:rightStart] == row2[:rightStart] {
		t.Errorf("left halves should NOT match because keys and values differ:\n  row1=%q\n  row2=%q", row1, row2)
	}
}

// ── FormatElapsed ─────────────────────────────────────────────────────────────

func TestFormatElapsed_Zero(t *testing.T) {
	got := terminal.FormatElapsed(0)
	if got != "00:00:00" {
		t.Errorf("FormatElapsed(0) = %q, want %q", got, "00:00:00")
	}
}

func TestFormatElapsed_Negative(t *testing.T) {
	got := terminal.FormatElapsed(-5 * time.Second)
	if got != "00:00:00" {
		t.Errorf("FormatElapsed(negative) = %q, want %q", got, "00:00:00")
	}
}

func TestFormatElapsed_OneHour(t *testing.T) {
	got := terminal.FormatElapsed(time.Hour + 2*time.Minute + 3*time.Second)
	if got != "01:02:03" {
		t.Errorf("FormatElapsed(1h2m3s) = %q, want %q", got, "01:02:03")
	}
}

func TestFormatElapsed_LargeHours(t *testing.T) {
	// 25 hours, 59 minutes, 59 seconds.
	d := 25*time.Hour + 59*time.Minute + 59*time.Second
	got := terminal.FormatElapsed(d)
	if got != "25:59:59" {
		t.Errorf("FormatElapsed(25h59m59s) = %q, want %q", got, "25:59:59")
	}
}

// ── FormatETA ─────────────────────────────────────────────────────────────────

func TestFormatETA_ZeroQueue(t *testing.T) {
	got := terminal.FormatETA(0, 100)
	if got != "-" {
		t.Errorf("FormatETA(0 queued, 100 rps) = %q, want \"-\"", got)
	}
}

func TestFormatETA_ZeroRPS(t *testing.T) {
	got := terminal.FormatETA(1000, 0)
	if got != "-" {
		t.Errorf("FormatETA(1000 queued, 0 rps) = %q, want \"-\"", got)
	}
}

func TestFormatETA_SubSecond(t *testing.T) {
	// 1 queued at 100/s → 0.01s → rounded up to 1s minimum.
	got := terminal.FormatETA(1, 100)
	if got != "00:00:01" {
		t.Errorf("FormatETA(sub-second) = %q, want %q", got, "00:00:01")
	}
}

func TestFormatETA_ExactMinute(t *testing.T) {
	// 6000 queued at 100/s → 60s → 00:01:00.
	got := terminal.FormatETA(6000, 100)
	if got != "00:01:00" {
		t.Errorf("FormatETA(60s) = %q, want %q", got, "00:01:00")
	}
}

func TestFormatETA_NeverNegativeRPS(t *testing.T) {
	got := terminal.FormatETA(100, -5)
	if got != "-" {
		t.Errorf("FormatETA(negative rps) = %q, want \"-\"", got)
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
	if !strings.Contains(got, "K ......................... V") {
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
