package output

import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/unsubble/searchit/internal/color"
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
	palette       *color.Palette
}

// NewTextFormatter creates a Formatter writing to io.Writer with colors disabled.
func NewTextFormatter(w io.Writer, quiet bool, showHeaders bool, showTitle bool, humanReadable bool) *TextFormatter {
	return NewColoredTextFormatter(w, quiet, showHeaders, showTitle, humanReadable, false)
}

// NewColoredTextFormatter creates a Formatter writing to io.Writer with configurable color support.
func NewColoredTextFormatter(w io.Writer, quiet bool, showHeaders bool, showTitle bool, humanReadable bool, useColor bool) *TextFormatter {
	return &TextFormatter{
		w:             w,
		quiet:         quiet,
		showHeaders:   showHeaders,
		showTitle:     showTitle,
		humanReadable: humanReadable,
		palette:       color.New(useColor),
	}
}

// SetColor updates the color status of the formatter.
func (f *TextFormatter) SetColor(useColor bool) {
	f.palette = color.New(useColor)
}

func (f *TextFormatter) Print(r engine.Result) error {
	return writeTextResult(f.w, r, f.quiet, f.showHeaders, f.showTitle, f.humanReadable, f.palette)
}

func (f *TextFormatter) PrintTo(w io.Writer, r engine.Result) error {
	return writeTextResult(w, r, f.quiet, f.showHeaders, f.showTitle, f.humanReadable, f.palette)
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
	palette       *color.Palette
}

// NewTerminalTextFormatter creates a Formatter writing via a TerminalManager with colors disabled.
func NewTerminalTextFormatter(tm *terminal.Manager, owner terminal.Owner, quiet bool, showHeaders bool, showTitle bool, humanReadable bool) *TerminalTextFormatter {
	return NewColoredTerminalTextFormatter(tm, owner, quiet, showHeaders, showTitle, humanReadable, false)
}

// NewColoredTerminalTextFormatter creates a Formatter writing via a TerminalManager with configurable color support.
func NewColoredTerminalTextFormatter(tm *terminal.Manager, owner terminal.Owner, quiet bool, showHeaders bool, showTitle bool, humanReadable bool, useColor bool) *TerminalTextFormatter {
	return &TerminalTextFormatter{
		tm:            tm,
		owner:         owner,
		quiet:         quiet,
		showHeaders:   showHeaders,
		showTitle:     showTitle,
		humanReadable: humanReadable,
		palette:       color.New(useColor),
	}
}

// SetColor updates the color status of the terminal formatter.
func (f *TerminalTextFormatter) SetColor(useColor bool) {
	f.palette = color.New(useColor)
}

func (f *TerminalTextFormatter) Print(r engine.Result) error {
	return f.tm.Emit(f.owner, func(w io.Writer) {
		_ = writeTextResult(w, r, f.quiet, f.showHeaders, f.showTitle, f.humanReadable, f.palette)
	})
}

func (f *TerminalTextFormatter) PrintTo(w io.Writer, r engine.Result) error {
	return writeTextResult(w, r, f.quiet, f.showHeaders, f.showTitle, f.humanReadable, f.palette)
}

func (f *TerminalTextFormatter) Close() error {
	return nil
}

// writeTextResult is the shared rendering logic.
func writeTextResult(w io.Writer, r engine.Result, quiet, showHeaders, showTitle, humanReadable bool, palette *color.Palette) error {
	if palette == nil {
		palette = color.New(false)
	}

	if quiet {
		_, err := fmt.Fprintf(w, "%s\n", palette.Finding(r.URL))
		return err
	}

	if showHeaders || showTitle {
		return writeVerboseTextResult(w, r, showHeaders, showTitle, humanReadable, palette)
	}

	if isRedirect(r) {
		return writeRedirectResult(w, r, humanReadable, palette)
	}

	if r.IsFuzz || r.Origin == "fuzz" || r.FuzzData != nil {
		return writeFuzzTextResult(w, r, humanReadable, palette)
	}

	return writeNormalTextResult(w, r, humanReadable, palette)
}

func hasNonURLFuzzFields(r engine.Result) bool {
	if r.FuzzData == nil || len(r.FuzzData.Fields) == 0 {
		return false
	}
	for _, f := range r.FuzzData.Fields {
		switch f.Location {
		case engine.LocationHeader, engine.LocationCookie, engine.LocationBody, engine.LocationJSON:
			return true
		}
	}
	return false
}

func writeFuzzTextResult(w io.Writer, r engine.Result, humanReadable bool, palette *color.Palette) error {
	if palette == nil {
		palette = color.New(false)
	}
	if !hasNonURLFuzzFields(r) {
		return writeNormalTextResult(w, r, humanReadable, palette)
	}

	var sb strings.Builder
	s := formatSize(r.Length, humanReadable, palette)
	sb.WriteString(fmt.Sprintf("%s %s - %s\n", palette.Success("[+]"), palette.Status(r.StatusCode), s))
	sb.WriteString(fmt.Sprintf("  %s\n", palette.Heading("URL")))
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
			sb.WriteString(fmt.Sprintf("  %s\n", palette.Heading("Header")))
			for _, h := range headers {
				if h.Name != "" {
					sb.WriteString(fmt.Sprintf("    %s: %s\n", h.Name, h.Value))
				} else {
					sb.WriteString(fmt.Sprintf("    %s\n", h.Value))
				}
			}
		}

		if len(cookies) > 0 {
			sb.WriteString(fmt.Sprintf("  %s\n", palette.Heading("Cookie")))
			for _, c := range cookies {
				if c.Name != "" {
					sb.WriteString(fmt.Sprintf("    %s=%s\n", c.Name, c.Value))
				} else {
					sb.WriteString(fmt.Sprintf("    %s\n", c.Value))
				}
			}
		}

		if len(bodies) > 0 {
			sb.WriteString(fmt.Sprintf("  %s\n", palette.Heading("Body")))
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

// FormatSize formats a byte length into a string representation without styling.
// When humanReadable is true, sizes are formatted in B, KB, MB, GB units (e.g. 9.6 KB, 1.0 MB).
// When humanReadable is false (the default), sizes are formatted as raw byte counts (e.g. 9797 B, 1048576 B).
// Unknown sizes (negative length) render as "? B" in both modes.
// Zero-byte sizes render as "0 B" in both modes.
func FormatSize(length int64, humanReadable bool) string {
	return FormatSizeStyled(length, humanReadable, nil)
}

// FormatSizeStyled formats a byte length into a string representation, optionally styling the
// byte received text (e.g. "313 B", "9.6 KB", "1.0 MB") in bold white using the provided palette.
func FormatSizeStyled(length int64, humanReadable bool, palette *color.Palette) string {
	var raw string
	if length < 0 {
		raw = "? B"
	} else if !humanReadable || length < 1024 {
		raw = fmt.Sprintf("%d B", length)
	} else if length < 1024*1024 {
		raw = fmt.Sprintf("%.1f KB", float64(length)/1024.0)
	} else if length < 1024*1024*1024 {
		raw = fmt.Sprintf("%.1f MB", float64(length)/(1024.0*1024.0))
	} else {
		raw = fmt.Sprintf("%.1f GB", float64(length)/(1024.0*1024.0*1024.0))
	}

	if palette == nil || !palette.Enabled() {
		return raw
	}
	return palette.ByteReceived(raw)
}

func formatSize(length int64, humanReadable bool, palette *color.Palette) string {
	return FormatSizeStyled(length, humanReadable, palette)
}

func writeRedirectResult(w io.Writer, r engine.Result, humanReadable bool, palette *color.Palette) error {
	if palette == nil {
		palette = color.New(false)
	}
	reqPath := requestedPath(r.URL)
	loc := ""
	if r.Headers != nil {
		loc = r.Headers.Get("Location")
	}
	if loc == "" {
		loc = r.RedirectURL
	}
	s := formatSize(r.Length, humanReadable, palette)
	arrow := "->"
	if palette.Enabled() {
		arrow = palette.BoldWhite("->")
	}
	_, err := fmt.Fprintf(w, "%s - %s - %s %s %s\n", palette.StatusBracketed(r.StatusCode), s, reqPath, arrow, loc)
	return err
}

func writeNormalTextResult(w io.Writer, r engine.Result, humanReadable bool, palette *color.Palette) error {
	if palette == nil {
		palette = color.New(false)
	}
	s := formatSize(r.Length, humanReadable, palette)
	_, err := fmt.Fprintf(w, "%s %s - %s - %s\n", palette.Success("[+]"), palette.Status(r.StatusCode), s, r.URL)
	return err
}

func writeVerboseTextResult(w io.Writer, r engine.Result, showHeaders, showTitle, humanReadable bool, palette *color.Palette) error {
	if palette == nil {
		palette = color.New(false)
	}
	var sb strings.Builder

	sizeStr := formatSize(r.Length, humanReadable, palette)

	sb.WriteString(fmt.Sprintf("%s     %s\n\n%s\n", palette.Status(r.StatusCode), sizeStr, r.URL))

	if showTitle {
		sb.WriteString("\n")
		sb.WriteString(palette.Heading("TITLE:"))
		sb.WriteString("\n------\n")
		if r.Title != "" {
			sb.WriteString(r.Title)
		}
		sb.WriteString("\n------\n")
	}

	if showHeaders && len(r.Headers) > 0 {
		sb.WriteString("\n")
		sb.WriteString(palette.Heading("HEADERS:"))
		sb.WriteString("\n--------\n")
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
