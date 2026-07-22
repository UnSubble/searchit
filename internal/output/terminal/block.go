package terminal

import (
	"fmt"
	"io"
	"strings"
)

const (
	// DefaultSeparatorChar is the separator character used by RenderBlock.
	DefaultSeparatorChar = "-"

	// ThinSeparatorChar is the box-drawing thin horizontal separator.
	ThinSeparatorChar = "─"

	// ThickSeparatorChar is the box-drawing double horizontal separator.
	ThickSeparatorChar = "═"
)

// Item represents a key-value pair in a formatted block.
// It is deliberately kept identical to output.Item so callers can convert
// trivially during the migration period.
type Item struct {
	Key   string
	Value string
}

// Separator returns a horizontal rule string of exactly width characters using char.
//   - If width ≤ 0, defaultWidth (80) is used.
func Separator(width int, char string) string {
	if width <= 0 {
		width = defaultWidth
	}
	if char == "" {
		char = DefaultSeparatorChar
	}
	return strings.Repeat(char, width)
}

// CenterTitle returns title centered within width using space padding.
// If title is longer than width, it is returned as-is.
func CenterTitle(title string, width int) string {
	if len(title) >= width {
		return title
	}
	padding := (width - len(title)) / 2
	if padding < 0 {
		padding = 0
	}
	return strings.Repeat(" ", padding) + title
}

// RenderBlock prints a titled, bordered block of dot-padded key-value rows to w.
// It uses the provided contentWidth and does NOT truncate rows (wrapping is allowed).
func RenderBlock(w io.Writer, title string, items []Item, contentWidth int) {
	RenderBlockWithWidth(w, title, items, contentWidth, 0)
}

// RenderBlockWithWidth prints a block with an explicitly provided width for the separator,
// and optionally truncates rows to maxColumns (if maxColumns > 0).
func RenderBlockWithWidth(w io.Writer, title string, items []Item, sepWidth, maxColumns int) {
	sep := strings.Repeat(DefaultSeparatorChar, sepWidth)

	fmt.Fprintln(w, sep)
	if title != "" {
		fmt.Fprintln(w, CenterTitle(title, sepWidth))
		fmt.Fprintln(w, sep)
	}
	for _, item := range items {
		fmt.Fprintln(w, FormatDotRow(item.Key, item.Value, 0, maxColumns))
	}
	fmt.Fprintln(w, sep)
}

// RenderSection prints a titled section with a thin box-drawing separator (─).
// It is used by the statistics full-screen view for section headers.
//
// Layout:
//
//	(blank)
//	Section Name
//	──────────────────────────────────────────────────────────────
//	(blank)
func RenderSection(w io.Writer, title string, contentWidth int) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, title)
	fmt.Fprintln(w, Separator(contentWidth, ThinSeparatorChar))
	fmt.Fprintln(w)
}
