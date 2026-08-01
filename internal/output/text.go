package output

import (
	"fmt"
	"io"
	"net/url"
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
		if err != nil {
			return err
		}
		return writeFuzzFieldsText(w, r.FuzzData)
	}

	if showHeaders || showTitle {
		if err := writeVerboseTextResult(w, r, showHeaders, showTitle); err != nil {
			return err
		}
		return writeFuzzFieldsText(w, r.FuzzData)
	}

	if isRedirect(r) {
		if err := writeRedirectResult(w, r); err != nil {
			return err
		}
		return writeFuzzFieldsText(w, r.FuzzData)
	}

	if err := writeNormalTextResult(w, r); err != nil {
		return err
	}
	return writeFuzzFieldsText(w, r.FuzzData)
}

func writeFuzzFieldsText(w io.Writer, fuzzData *engine.FuzzData) error {
	if fuzzData == nil || len(fuzzData.Fields) == 0 {
		return nil
	}
	for _, field := range fuzzData.Fields {
		switch field.Location {
		case engine.LocationHeader:
			if field.Name != "" {
				if _, err := fmt.Fprintf(w, "Header: %s=%s\n", field.Name, field.Value); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "Header: %s\n", field.Value); err != nil {
					return err
				}
			}
		case engine.LocationCookie:
			if field.Name != "" {
				if _, err := fmt.Fprintf(w, "Cookie: %s=%s\n", field.Name, field.Value); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "Cookie: %s\n", field.Value); err != nil {
					return err
				}
			}
		case engine.LocationBody:
			if _, err := fmt.Fprintf(w, "Body: %s\n", field.Value); err != nil {
				return err
			}
		case engine.LocationJSON:
			if _, err := fmt.Fprintf(w, "JSON: %s\n", field.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

func isRedirect(r engine.Result) bool {
	switch r.StatusCode {
	case 300, 301, 302, 303, 307, 308:
		return (r.Headers != nil && r.Headers.Get("Location") != "") || r.RedirectURL != ""
	default:
		return false
	}
}

func requestedPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	reqURI := u.RequestURI()
	if reqURI == "" {
		return rawURL
	}
	return reqURI
}

func formatSize(length int64) string {
	if length < 1024 {
		return fmt.Sprintf("%d B", length)
	}
	if length < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(length)/1024.0)
	}
	if length < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(length)/(1024.0*1024.0))
	}
	return fmt.Sprintf("%.1f GB", float64(length)/(1024.0*1024.0*1024.0))
}

func writeRedirectResult(w io.Writer, r engine.Result) error {
	reqPath := requestedPath(r.URL)
	loc := ""
	if r.Headers != nil {
		loc = r.Headers.Get("Location")
	}
	if loc == "" {
		loc = r.RedirectURL
	}
	if r.Length >= 0 {
		s := formatSize(r.Length)
		_, err := fmt.Fprintf(w, "[%d] - %s - %s -> %s\n", r.StatusCode, s, reqPath, loc)
		return err
	}
	_, err := fmt.Fprintf(w, "[%d] -        - %s -> %s\n", r.StatusCode, reqPath, loc)
	return err
}

func writeNormalTextResult(w io.Writer, r engine.Result) error {
	if r.Length >= 0 {
		s := formatSize(r.Length)
		_, err := fmt.Fprintf(w, "[+] %d - %s - %s\n", r.StatusCode, s, r.URL)
		return err
	}
	_, err := fmt.Fprintf(w, "[+] %d -        - %s\n", r.StatusCode, r.URL)
	return err
}

func writeVerboseTextResult(w io.Writer, r engine.Result, showHeaders, showTitle bool) error {
	var sb strings.Builder

	sizeStr := "0 B"
	if r.Length >= 0 {
		sizeStr = fmt.Sprintf("%d B", r.Length)
	} else {
		sizeStr = "-1 B"
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
