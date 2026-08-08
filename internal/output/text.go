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
	w             io.Writer
	quiet         bool
	showHeaders   bool
	showTitle     bool
	humanReadable bool
}

// NewTextFormatter creates a Formatter writing to io.Writer.
func NewTextFormatter(w io.Writer, quiet bool, showHeaders bool, showTitle bool, humanReadable bool) *TextFormatter {
	return &TextFormatter{
		w:             w,
		quiet:         quiet,
		showHeaders:   showHeaders,
		showTitle:     showTitle,
		humanReadable: humanReadable,
	}
}

func (f *TextFormatter) Print(r engine.Result) error {
	return writeTextResult(f.w, r, f.quiet, f.showHeaders, f.showTitle, f.humanReadable)
}

func (f *TextFormatter) PrintTo(w io.Writer, r engine.Result) error {
	return writeTextResult(w, r, f.quiet, f.showHeaders, f.showTitle, f.humanReadable)
}

func (f *TextFormatter) Close() error {
	return nil
}

// TerminalTextFormatter writes clean text through a TerminalManager.
type TerminalTextFormatter struct {
	tm            *terminal.Manager
	owner         terminal.Owner
	quiet         bool
	showHeaders   bool
	showTitle     bool
	humanReadable bool
}

// NewTerminalTextFormatter creates a Formatter writing via a TerminalManager.
func NewTerminalTextFormatter(tm *terminal.Manager, owner terminal.Owner, quiet bool, showHeaders bool, showTitle bool, humanReadable bool) *TerminalTextFormatter {
	return &TerminalTextFormatter{
		tm:            tm,
		owner:         owner,
		quiet:         quiet,
		showHeaders:   showHeaders,
		showTitle:     showTitle,
		humanReadable: humanReadable,
	}
}

func (f *TerminalTextFormatter) Print(r engine.Result) error {
	return f.tm.Emit(f.owner, func(w io.Writer) {
		_ = writeTextResult(w, r, f.quiet, f.showHeaders, f.showTitle, f.humanReadable)
	})
}

func (f *TerminalTextFormatter) PrintTo(w io.Writer, r engine.Result) error {
	return writeTextResult(w, r, f.quiet, f.showHeaders, f.showTitle, f.humanReadable)
}

func (f *TerminalTextFormatter) Close() error {
	return nil
}

// writeTextResult is the shared rendering logic.
func writeTextResult(w io.Writer, r engine.Result, quiet, showHeaders, showTitle, humanReadable bool) error {
	if quiet {
		_, err := fmt.Fprintf(w, "%s\n", r.URL)
		return err
	}

	if showHeaders || showTitle {
		return writeVerboseTextResult(w, r, showHeaders, showTitle, humanReadable)
	}

	if isRedirect(r) {
		return writeRedirectResult(w, r, humanReadable)
	}

	if r.IsFuzz || r.Origin == "fuzz" || r.FuzzData != nil {
		return writeFuzzTextResult(w, r, humanReadable)
	}

	return writeNormalTextResult(w, r, humanReadable)
}

func writeFuzzTextResult(w io.Writer, r engine.Result, humanReadable bool) error {
	var sb strings.Builder
	s := formatSize(r.Length, humanReadable)
	sb.WriteString(fmt.Sprintf("[+] %d - %s\n", r.StatusCode, s))
	sb.WriteString("  URL\n")
	sb.WriteString(fmt.Sprintf("    %s\n", r.URL))

	if r.FuzzData != nil && len(r.FuzzData.Fields) > 0 {
		var headers, cookies, bodies []engine.FuzzField
		for _, f := range r.FuzzData.Fields {
			switch f.Location {
			case engine.LocationHeader:
				headers = append(headers, f)
			case engine.LocationCookie:
				cookies = append(cookies, f)
			case engine.LocationBody, engine.LocationJSON:
				bodies = append(bodies, f)
			}
		}

		if len(headers) > 0 {
			sb.WriteString("  Header\n")
			for _, h := range headers {
				if h.Name != "" {
					sb.WriteString(fmt.Sprintf("    %s: %s\n", h.Name, h.Value))
				} else {
					sb.WriteString(fmt.Sprintf("    %s\n", h.Value))
				}
			}
		}

		if len(cookies) > 0 {
			sb.WriteString("  Cookie\n")
			for _, c := range cookies {
				if c.Name != "" {
					sb.WriteString(fmt.Sprintf("    %s=%s\n", c.Name, c.Value))
				} else {
					sb.WriteString(fmt.Sprintf("    %s\n", c.Value))
				}
			}
		}

		if len(bodies) > 0 {
			sb.WriteString("  Body\n")
			for _, b := range bodies {
				lines := strings.Split(b.Value, "\n")
				for _, line := range lines {
					sb.WriteString(fmt.Sprintf("    %s\n", line))
				}
			}
		}
	}

	sb.WriteString("\n")
	_, err := io.WriteString(w, sb.String())
	return err
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

// FormatSize formats a byte length into a string representation.
// When humanReadable is true, sizes are formatted in B, KB, MB, GB units (e.g. 9.6 KB, 1.0 MB).
// When humanReadable is false (the default), sizes are formatted as raw byte counts (e.g. 9797 B, 1048576 B).
// Unknown sizes (negative length) render as "? B" in both modes.
// Zero-byte sizes render as "0 B" in both modes.
func FormatSize(length int64, humanReadable bool) string {
	if length < 0 {
		return "? B"
	}
	if !humanReadable {
		return fmt.Sprintf("%d B", length)
	}
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

func formatSize(length int64, humanReadable bool) string {
	return FormatSize(length, humanReadable)
}

func writeRedirectResult(w io.Writer, r engine.Result, humanReadable bool) error {
	reqPath := requestedPath(r.URL)
	loc := ""
	if r.Headers != nil {
		loc = r.Headers.Get("Location")
	}
	if loc == "" {
		loc = r.RedirectURL
	}
	s := formatSize(r.Length, humanReadable)
	_, err := fmt.Fprintf(w, "[%d] - %s - %s -> %s\n", r.StatusCode, s, reqPath, loc)
	return err
}

func writeNormalTextResult(w io.Writer, r engine.Result, humanReadable bool) error {
	s := formatSize(r.Length, humanReadable)
	_, err := fmt.Fprintf(w, "[+] %d - %s - %s\n", r.StatusCode, s, r.URL)
	return err
}

func writeVerboseTextResult(w io.Writer, r engine.Result, showHeaders, showTitle, humanReadable bool) error {
	var sb strings.Builder

	sizeStr := formatSize(r.Length, humanReadable)

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
