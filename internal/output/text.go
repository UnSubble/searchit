package output

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output/terminal"
)

// TextFormatter writes clean text.
type TextFormatter struct {
	w           io.Writer
	quiet       bool
	showHeaders bool
	showTitle   bool
}

// NewTextFormatter creates a Formatter writing to io.Writer.
func NewTextFormatter(w io.Writer, quiet bool, showHeaders bool, showTitle bool) *TextFormatter {
	return &TextFormatter{w: w, quiet: quiet, showHeaders: showHeaders, showTitle: showTitle}
}

func (f *TextFormatter) Print(r engine.Result) error {
	return writeTextResult(f.w, r, f.quiet, f.showHeaders, f.showTitle)
}

func (f *TextFormatter) PrintTo(w io.Writer, r engine.Result) error {
	return writeTextResult(w, r, f.quiet, f.showHeaders, f.showTitle)
}

func (f *TextFormatter) Close() error {
	return nil
}

// TerminalTextFormatter writes clean text through a TerminalManager.
type TerminalTextFormatter struct {
	tm          *terminal.Manager
	owner       terminal.Owner
	quiet       bool
	showHeaders bool
	showTitle   bool
}

// NewTerminalTextFormatter creates a Formatter writing via a TerminalManager.
func NewTerminalTextFormatter(tm *terminal.Manager, owner terminal.Owner, quiet bool, showHeaders bool, showTitle bool) *TerminalTextFormatter {
	return &TerminalTextFormatter{
		tm:          tm,
		owner:       owner,
		quiet:       quiet,
		showHeaders: showHeaders,
		showTitle:   showTitle,
	}
}

func (f *TerminalTextFormatter) Print(r engine.Result) error {
	return f.tm.Emit(f.owner, func(w io.Writer) {
		_ = writeTextResult(w, r, f.quiet, f.showHeaders, f.showTitle)
	})
}

func (f *TerminalTextFormatter) PrintTo(w io.Writer, r engine.Result) error {
	return writeTextResult(w, r, f.quiet, f.showHeaders, f.showTitle)
}

func (f *TerminalTextFormatter) Close() error {
	return nil
}

// writeTextResult is the shared rendering logic.
func writeTextResult(w io.Writer, r engine.Result, quiet, showHeaders, showTitle bool) error {
	if quiet {
		_, err := fmt.Fprintf(w, "%s\n", r.URL)
		return err
	}

	if showHeaders || showTitle {
		var sb strings.Builder

		sizeStr := "0B"
		if r.Length >= 0 {
			sizeStr = fmt.Sprintf("%dB", r.Length)
		} else {
			sizeStr = "-1B"
		}

		sb.WriteString(fmt.Sprintf("%d     %s\n\n%s\n", r.StatusCode, sizeStr, r.URL))

		if showTitle {
			sb.WriteString("\nTITLE:\n------\n")
			if r.Title != "" {
				sb.WriteString(r.Title)
			}
			sb.WriteString("\n------\n")
		}

		if showHeaders && len(r.Headers) > 0 {
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

		_, err := io.WriteString(w, sb.String())
		return err
	}

	if r.Length >= 0 {
		var s string
		if r.Length < 1024 {
			s = fmt.Sprintf("%dB", r.Length)
		} else if r.Length < 1024*1024 {
			s = fmt.Sprintf("%.1fKB", float64(r.Length)/1024.0)
		} else if r.Length < 1024*1024*1024 {
			s = fmt.Sprintf("%.1fMB", float64(r.Length)/(1024.0*1024.0))
		} else {
			s = fmt.Sprintf("%.1fGB", float64(r.Length)/(1024.0*1024.0*1024.0))
		}
		_, err := fmt.Fprintf(w, "[+] %d - %s - %s\n", r.StatusCode, s, r.URL)
		return err
	}
	_, err := fmt.Fprintf(w, "[+] %d -        - %s\n", r.StatusCode, r.URL)
	return err
}
