package terminal

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	// DefaultPadWidth is the column at which dot padding ends and values begin.
	// Matches the current output.KeyPadWidth = 28.
	DefaultPadWidth = 28

	// TwoColLeftKeyWidth is the fixed width of the left key column in a two-column row.
	TwoColLeftKeyWidth = 12

	// TwoColLeftValWidth is the fixed width of the left value column in a two-column row.
	TwoColLeftValWidth = 14

	// TwoColRightKeyWidth is the fixed width of the right key column in a two-column row.
	TwoColRightKeyWidth = 12
)

// FormatDotRow formats a single key-value pair with dot padding.
//
// padWidth is the column at which the dots end; if ≤ 0, DefaultPadWidth is used.
// maxColumns truncates the output to maxColumns characters if > 0.
//
// Example (padWidth=28):
//
//	"Candidates .................. 5000"
func FormatDotRow(key, val string, padWidth, maxColumns int) string {
	if padWidth <= 0 {
		padWidth = DefaultPadWidth
	}
	// Clamp padWidth if the caller provided a narrow maxColumns.
	if maxColumns > 0 && maxColumns < padWidth+6 {
		padWidth = maxColumns / 2
	}

	var line string
	if len(key) >= padWidth-2 {
		// Key is too long for dot padding — use a short ellipsis separator.
		line = fmt.Sprintf("%s .. %s", key, val)
	} else {
		dotsCount := padWidth - len(key) - 2
		if dotsCount < 1 {
			dotsCount = 1
		}
		line = fmt.Sprintf("%s %s %s", key, strings.Repeat(".", dotsCount), val)
	}

	if maxColumns > 0 && len(line) > maxColumns {
		return line[:maxColumns]
	}
	return line
}

// FormatTwoColumnRow formats two key-value pairs into a fixed two-column layout.
//
// Column widths:
//
//	left key  = TwoColLeftKeyWidth  (12)
//	left val  = TwoColLeftValWidth  (14)
//	right key = TwoColRightKeyWidth (12)
//	right val = remainder
//
// Example:
//
//	"Elapsed     00:00:03      ETA         00:01:22"
func FormatTwoColumnRow(leftKey, leftVal, rightKey, rightVal string) string {
	leftText := fmt.Sprintf("%-*s%-*s", TwoColLeftKeyWidth, leftKey, TwoColLeftValWidth, leftVal)
	rightText := fmt.Sprintf("%-*s%s", TwoColRightKeyWidth, rightKey, rightVal)
	return leftText + rightText
}

// FormatElapsed formats a time.Duration as "HH:MM:SS".
//
// This replaces the two independent implementations that previously existed in
// internal/progress/ansi.go and internal/progress/stats_view.go.
func FormatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// FormatETA returns a formatted "HH:MM:SS" ETA string.
// Returns "-" when the ETA cannot be computed (rps ≤ 0 or queued == 0).
// Enforces a minimum of 1 second when queued > 0 and rps > 0.
func FormatETA(queued int64, rps float64) string {
	if rps <= 0 || queued <= 0 {
		return "-"
	}
	etaSecs := float64(queued) / rps
	if etaSecs < 1.0 {
		etaSecs = 1.0
	}
	total := int(math.Ceil(etaSecs))
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// FormatLatency formats a latency duration into a human-readable string.
// Returns "-" for zero or negative durations.
func FormatLatency(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}
