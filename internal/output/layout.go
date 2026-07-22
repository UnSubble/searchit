package output

import (
	"fmt"
	"io"
	"strings"
)

const (
	// DefaultLineWidth is the standard 56-character separator width.
	DefaultLineWidth = 56
	// KeyPadWidth is the column index where dot padding ends and values begin.
	KeyPadWidth = 28
)

// Item represents a key-value row in a formatted block.
type Item struct {
	Key   string
	Value string
}

// FormatRow formats a key-value pair into a dot-padded string bounded by maxColumns (if maxColumns > 0).
func FormatRow(key, val string, maxColumns int) string {
	padWidth := KeyPadWidth
	if maxColumns > 0 && maxColumns < padWidth+6 {
		padWidth = maxColumns / 2
	}

	var line string
	if len(key) >= padWidth-2 {
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

// FormatTwoColumnRow formats two key-value pairs into a clean 2-column layout.
func FormatTwoColumnRow(leftKey, leftVal, rightKey, rightVal string, maxColumns int) string {
	leftText := fmt.Sprintf("%-12s%-14s", leftKey, leftVal)
	rightText := fmt.Sprintf("%-12s%s", rightKey, rightVal)
	line := leftText + rightText

	if maxColumns > 0 && len(line) > maxColumns {
		return line[:maxColumns]
	}
	return line
}

// RenderDivider prints a divider line of specified character and width.
func RenderDivider(w io.Writer, width int, char string) {
	if width <= 0 {
		width = DefaultLineWidth
	}
	if char == "" {
		char = "-"
	}
	fmt.Fprintln(w, strings.Repeat(char, width))
}

// RenderBlock prints a left-aligned, dot-padded section block with separators.
func RenderBlock(w io.Writer, title string, items []Item, maxColumns int) {
	width := DefaultLineWidth
	if maxColumns > 0 {
		width = maxColumns
	}

	sep := strings.Repeat("-", width)
	fmt.Fprintln(w, sep)

	if title != "" {
		padding := (width - len(title)) / 2
		if padding < 0 {
			padding = 0
		}
		fmt.Fprintf(w, "%s%s\n", strings.Repeat(" ", padding), title)
		fmt.Fprintln(w, sep)
	}

	for _, item := range items {
		fmt.Fprintln(w, FormatRow(item.Key, item.Value, maxColumns))
	}

	fmt.Fprintln(w, sep)
}
