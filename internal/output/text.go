package output

import (
	"fmt"
	"io"

	"github.com/unsubble/searchit/internal/engine"
)

type TextFormatter struct {
	w     io.Writer
	quiet bool
}

func NewTextFormatter(w io.Writer, quiet bool) *TextFormatter {
	return &TextFormatter{w: w, quiet: quiet}
}

func (f *TextFormatter) Print(r engine.Result) error {
	if f.quiet {
		_, err := fmt.Fprintf(f.w, "%s\n", r.URL)
		return err
	}
	_, err := fmt.Fprintf(f.w, "[+] %d - %s\n", r.StatusCode, r.URL)
	return err
}

func (f *TextFormatter) Close() error {
	return nil
}
