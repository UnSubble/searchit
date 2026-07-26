package terminal

import (
	"fmt"
	"time"
)

const (
	// DefaultPadWidth is the column at which dot padding ends and values begin.
	// Matches the current output.KeyPadWidth = 28.
	DefaultPadWidth = 28

	// TwoColLeftKeyWidth is the fixed width of the left key column in a two-column row.
	TwoColLeftKeyWidth = 14

	// TwoColLeftValWidth is the fixed width of the left value column in a two-column row.
	TwoColLeftValWidth = 12

	// TwoColRightKeyWidth is the fixed width of the right key column in a two-column row.
	TwoColRightKeyWidth = 12
)

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
