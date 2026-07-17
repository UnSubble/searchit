package output

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/unsubble/searchit/internal/engine"
)

// MarkdownFormatter writes scan results as a GitHub-compatible Markdown table.
// The header and separator rows are written on the first Print call;
// subsequent calls append data rows immediately (streaming).
type MarkdownFormatter struct {
	w           io.Writer
	hasHeader   bool
	showHeaders bool
	showTitle   bool
}

func NewMarkdownFormatter(w io.Writer, showHeaders bool, showTitle bool) *MarkdownFormatter {
	return &MarkdownFormatter{w: w, showHeaders: showHeaders, showTitle: showTitle}
}

func (f *MarkdownFormatter) Print(r engine.Result) error {
	if !f.hasHeader {
		header := "| URL | Status | Length | Depth |"
		sep := "| --- | -----: | -----: | ----: |"
		if f.showTitle {
			header += " Title |"
			sep += " --- |"
		}
		if f.showHeaders {
			header += " Headers |"
			sep += " --- |"
		}
		if _, err := fmt.Fprintln(f.w, header); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(f.w, sep); err != nil {
			return err
		}
		f.hasHeader = true
	}
	row := fmt.Sprintf("| %s | %d | %d | %d |", r.URL, r.StatusCode, r.Length, r.Depth)
	if f.showTitle {
		row += fmt.Sprintf(" %s |", r.Title)
	}
	if f.showHeaders {
		row += fmt.Sprintf(" %s |", formatMarkdownHeaders(r.Headers))
	}
	_, err := fmt.Fprintln(f.w, row)
	return err
}

func formatMarkdownHeaders(h http.Header) string {
	var parts []string
	var keys []string
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range h[k] {
			parts = append(parts, fmt.Sprintf("%s: %s", k, v))
		}
	}
	return strings.Join(parts, "; ")
}

func (f *MarkdownFormatter) Close() error {
	return nil
}
