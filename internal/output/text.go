package output

import (
	"fmt"
	"io"

	"github.com/unsubble/searchit/internal/engine"
)

type TextFormatter struct {
	w io.Writer
}

func NewTextFormatter(w io.Writer) *TextFormatter {
	return &TextFormatter{w: w}
}

func (f *TextFormatter) Print(r engine.Result) error {
	_, err := fmt.Fprintf(f.w, "[+] %d - %s\n", r.StatusCode, r.URL)
	return err
}

func (f *TextFormatter) Close() error {
	return nil
}
