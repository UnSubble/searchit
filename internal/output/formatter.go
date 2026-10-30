package output

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output/terminal"
)

// Formatter abstracts output presentation to decouple CLI presentation from the scanning engine.
type Formatter interface {
	// Print writes r to the formatter's configured output.
	Print(engine.Result) error
	// PrintTo writes r to w instead of the formatter's default output.
	// Used by ExecuteAbove to route finding output through the TM-locked writer,
	// ensuring all terminal writes (ANSI sequences + findings) share the same fd.
	PrintTo(io.Writer, engine.Result) error
	Close() error
}

// Format is the canonical name for an output formatter.
type Format string

const (
	FormatText     Format = "text"
	FormatJSON     Format = "json"
	FormatNDJSON   Format = "ndjson"
	FormatCSV      Format = "csv"
	FormatMarkdown Format = "markdown"
)

// supportedFormats is the single source of truth for all valid formatter names.
var supportedFormats = []Format{
	FormatText,
	FormatJSON,
	FormatNDJSON,
	FormatCSV,
	FormatMarkdown,
}

// extensionMap maps lowercase file extensions (without leading dot) to Format.
var extensionMap = map[string]Format{
	"txt":      FormatText,
	"text":     FormatText,
	"json":     FormatJSON,
	"ndjson":   FormatNDJSON,
	"csv":      FormatCSV,
	"md":       FormatMarkdown,
	"markdown": FormatMarkdown,
}

// SupportedFormats returns a sorted slice of all supported format name strings.
func SupportedFormats() []string {
	out := make([]string, len(supportedFormats))
	for i, f := range supportedFormats {
		out[i] = string(f)
	}
	return out
}

// Parse parses a format name string and returns the corresponding Format.
// It returns an error if the name is not recognised.
func Parse(s string) (Format, error) {
	f := Format(strings.ToLower(strings.TrimSpace(s)))
	for _, known := range supportedFormats {
		if f == known {
			return known, nil
		}
	}
	return "", fmt.Errorf("unsupported format %q: must be one of %s",
		s, strings.Join(SupportedFormats(), ", "))
}

// FormatFromExtension derives a Format from a filename extension.
// The ext argument should be a file extension as returned by filepath.Ext,
// i.e. it may or may not include a leading dot.
// Unknown extensions fall back to FormatText.
func FormatFromExtension(ext string) Format {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	if f, ok := extensionMap[ext]; ok {
		return f
	}
	return FormatText
}

// FormatFromPath derives a Format from a file path by inspecting its extension.
// Unknown extensions fall back to FormatText.
func FormatFromPath(path string) Format {
	return FormatFromExtension(filepath.Ext(path))
}

// NewColored constructs the appropriate Formatter for fmt with configurable color support,
// writing directly to w.
// Structured output formats (JSON, NDJSON, CSV, Markdown) never emit colors.
func NewColored(f Format, w io.Writer, quiet bool, showHeaders bool, showTitle bool, humanReadable bool, useColor bool) Formatter {
	switch f {
	case FormatJSON:
		return NewJSONFormatter(w, showHeaders, showTitle)
	case FormatNDJSON:
		return NewNDJSONFormatter(w, showHeaders, showTitle)
	case FormatCSV:
		return NewCSVFormatter(w, showHeaders, showTitle)
	case FormatMarkdown:
		return NewMarkdownFormatter(w, showHeaders, showTitle)
	default:
		return NewColoredTextFormatter(w, quiet, showHeaders, showTitle, humanReadable, useColor)
	}
}

// New constructs the appropriate Formatter for fmt, writing directly to w.
// Use this for writing to files or secondary outputs (colors are disabled).
func New(f Format, w io.Writer, quiet bool, showHeaders bool, showTitle bool, humanReadable bool) Formatter {
	return NewColored(f, w, quiet, showHeaders, showTitle, humanReadable, false)
}

// NewWithManagerColored constructs a Formatter with color support writing through the TerminalManager.
// Use this for stdout formatting so it respects the global output lock.
func NewWithManagerColored(f Format, tm *terminal.Manager, owner terminal.Owner, quiet bool, showHeaders bool, showTitle bool, humanReadable bool, useColor bool) Formatter {
	switch f {
	case FormatText:
		return NewColoredTerminalTextFormatter(tm, owner, quiet, showHeaders, showTitle, humanReadable, useColor)
	default:
		return NewColored(f, newTerminalAdapter(tm, owner), quiet, showHeaders, showTitle, humanReadable, useColor)
	}
}

// NewWithManager constructs a Formatter that writes through the TerminalManager with colors disabled.
// Use this for stdout formatting so it respects the global output lock.
// Currently only FormatText supports this directly via TerminalTextFormatter.
func NewWithManager(f Format, tm *terminal.Manager, owner terminal.Owner, quiet bool, showHeaders bool, showTitle bool, humanReadable bool) Formatter {
	return NewWithManagerColored(f, tm, owner, quiet, showHeaders, showTitle, humanReadable, false)
}

// terminalAdapter wraps a Manager to implement io.Writer for legacy formatters.
type terminalAdapter struct {
	tm    *terminal.Manager
	owner terminal.Owner
}

func newTerminalAdapter(tm *terminal.Manager, owner terminal.Owner) io.Writer {
	return &terminalAdapter{tm: tm, owner: owner}
}

func (t *terminalAdapter) Write(p []byte) (n int, err error) {
	// Must capture 'p' safely because Emit's closure runs synchronously.
	// But note: io.Writer contract allows synchronous write, so we can just pass it.
	_ = t.tm.Emit(t.owner, func(w io.Writer) {
		n, err = w.Write(p)
	})
	return
}
