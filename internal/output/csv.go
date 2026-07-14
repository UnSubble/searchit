package output

import (
	"encoding/csv"
	"io"

	"github.com/unsubble/searchit/internal/engine"
)

// CSVFormatter writes scan results as RFC 4180 CSV, one row per result.
// A header row is always written on the first Print call (streaming).
type CSVFormatter struct {
	w       *csv.Writer
	hasRows bool
}

func NewCSVFormatter(w io.Writer) *CSVFormatter {
	cw := csv.NewWriter(w)
	return &CSVFormatter{w: cw}
}

func (f *CSVFormatter) Print(r engine.Result) error {
	if !f.hasRows {
		if err := f.w.Write([]string{"url", "status", "length", "depth"}); err != nil {
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
	if err := f.w.Write(row); err != nil {
		return err
	}
	f.w.Flush()
	return f.w.Error()
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
