package output

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/unsubble/searchit/internal/engine"
)

// CSVFormatter writes scan results as RFC 4180 CSV, one row per result.
// A header row is always written on the first Print call (streaming).
type CSVFormatter struct {
	w           *csv.Writer
	hasRows     bool
	showHeaders bool
	showTitle   bool
}

func NewCSVFormatter(w io.Writer, showHeaders bool, showTitle bool) *CSVFormatter {
	cw := csv.NewWriter(w)
	return &CSVFormatter{w: cw, showHeaders: showHeaders, showTitle: showTitle}
}

func (f *CSVFormatter) Print(r engine.Result) error {
	if !f.hasRows {
		header := []string{"url", "status", "length", "depth"}
		if f.showTitle {
			header = append(header, "title")
		}
		if f.showHeaders {
			header = append(header, "headers")
		}
		if err := f.w.Write(header); err != nil {
			return err
		}
		f.hasRows = true
	}
	row := []string{
		r.URL,
		itoa(int64(r.StatusCode)),
		itoa(r.Length),
		itoa(int64(r.Depth)),
	}
	if f.showTitle {
		row = append(row, r.Title)
	}
	if f.showHeaders {
		row = append(row, formatCSVHeaders(r.Headers))
	}
	if err := f.w.Write(row); err != nil {
		return err
	}
	f.w.Flush()
	return f.w.Error()
}

func formatCSVHeaders(h http.Header) string {
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

func (f *CSVFormatter) Close() error {
	f.w.Flush()
	return f.w.Error()
}

// itoa converts an int64 to its decimal string representation without importing strconv.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
