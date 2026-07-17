package output

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/unsubble/searchit/internal/engine"
)

type TextFormatter struct {
	w           io.Writer
	quiet       bool
	showHeaders bool
	showTitle   bool
}

func NewTextFormatter(w io.Writer, quiet bool, showHeaders bool, showTitle bool) *TextFormatter {
	return &TextFormatter{w: w, quiet: quiet, showHeaders: showHeaders, showTitle: showTitle}
}

func (f *TextFormatter) Print(r engine.Result) error {
	if f.quiet {
		_, err := fmt.Fprintf(f.w, "%s\n", r.URL)
		return err
	}

	if f.showHeaders || f.showTitle {
		var sb strings.Builder

		sizeStr := "0B"
		if r.Length >= 0 {
			sizeStr = fmt.Sprintf("%dB", r.Length)
		} else {
			sizeStr = "-1B"
		}

		sb.WriteString(fmt.Sprintf("%d     %s\n\n%s\n", r.StatusCode, sizeStr, r.URL))

		if f.showTitle {
			sb.WriteString("\nTITLE:\n------\n")
			if r.Title != "" {
				sb.WriteString(r.Title)
			}
			sb.WriteString("\n------\n")
		}

		if f.showHeaders && len(r.Headers) > 0 {
			sb.WriteString("\nHEADERS:\n--------\n")
			var keys []string
			for k := range r.Headers {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				for _, v := range r.Headers[k] {
					sb.WriteString(fmt.Sprintf("%s: %s\n", k, v))
				}
			}
			sb.WriteString("--------\n")
		}

		_, err := io.WriteString(f.w, sb.String())
		return err
	}

	_, err := fmt.Fprintf(f.w, "[+] %d - %s\n", r.StatusCode, r.URL)
	return err
}

func (f *TextFormatter) Close() error {
	return nil
}
