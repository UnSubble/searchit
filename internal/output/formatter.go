package output

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/unsubble/searchit/internal/engine"
)

// Formatter abstracts output presentation to decouple CLI presentation from the scanning engine.
type Formatter interface {
	Print(engine.Result) error
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

// New constructs the appropriate Formatter for fmt, writing to w.
// For FormatText the quiet flag controls whether the status prefix is omitted.
func New(f Format, w io.Writer, quiet bool, showHeaders bool, showTitle bool) Formatter {
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
		return NewTextFormatter(w, quiet, showHeaders, showTitle)
	}
}
