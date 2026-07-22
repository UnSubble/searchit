package terminal

import (
	"io"
	"os"

	"github.com/unsubble/searchit/internal/console"
	"golang.org/x/term"
)

const (
	// defaultWidth is the fallback terminal width used when the actual
	// terminal width cannot be determined (non-terminal writers, errors).
	defaultWidth = 80

	// contentMargin is subtracted from the terminal width before clamping.
	contentMargin = 8

	// contentMinWidth is the minimum width for content after clamping.
	contentMinWidth = 72

	// contentMaxWidth is the maximum width for content after clamping.
	contentMaxWidth = 96
)

// Width calculates the current terminal column count for the given writer.
func width(w io.Writer) int {
	if f, ok := w.(*os.File); ok && console.IsTerminal(f.Fd()) {
		if cols, _, err := term.GetSize(int(f.Fd())); err == nil && cols > 0 {
			return cols
		}
	}
	return defaultWidth
}

// contentWidth computes: clamp(width(w) - contentMargin, contentMinWidth, contentMaxWidth).
func contentWidth(w io.Writer) int {
	return clamp(width(w)-contentMargin, contentMinWidth, contentMaxWidth)
}

// clamp constrains v to [lo, hi].
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
