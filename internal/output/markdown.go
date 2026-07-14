package output

import (
	"fmt"
	"io"

	"github.com/unsubble/searchit/internal/engine"
)

// MarkdownFormatter writes scan results as a GitHub-compatible Markdown table.
// The header and separator rows are written on the first Print call;
// subsequent calls append data rows immediately (streaming).
type MarkdownFormatter struct {
	w         io.Writer
	hasHeader bool
}

func NewMarkdownFormatter(w io.Writer) *MarkdownFormatter {
	return &MarkdownFormatter{w: w}
}

func (f *MarkdownFormatter) Print(r engine.Result) error {
	if !f.hasHeader {
		if _, err := fmt.Fprintln(f.w, "| URL | Status | Length | Depth |"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(f.w, "| --- | -----: | -----: | ----: |"); err != nil {
			return err
		}
		f.hasHeader = true
	}
	_, err := fmt.Fprintf(f.w,
		"| %s | %d | %d | %d |\n",
		r.URL, r.StatusCode, r.Length, r.Depth,
	)
	return err
}

func (f *MarkdownFormatter) Close() error {
	return nil
}
