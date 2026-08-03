package output

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/unsubble/searchit/internal/engine"
)

type JSONFormatter struct {
	w           io.Writer
	showHeaders bool
	showTitle   bool
	hasPrinted  bool
	closed      bool
}

type jsonResult struct {
	URL     string             `json:"url"`
	Status  int                `json:"status"`
	Length  int64              `json:"length"`
	Depth   uint16             `json:"depth"`
	Title   string             `json:"title,omitempty"`
	Headers http.Header        `json:"headers,omitempty"`
	Fuzz    []engine.FuzzField `json:"fuzz,omitempty"`
}

func NewJSONFormatter(w io.Writer, showHeaders bool, showTitle bool) *JSONFormatter {
	return &JSONFormatter{w: w, showHeaders: showHeaders, showTitle: showTitle}
}

func (f *JSONFormatter) Print(r engine.Result) error {
	jr := jsonResult{
		URL:    r.URL,
		Status: r.StatusCode,
		Length: r.Length,
		Depth:  r.Depth,
	}
	if f.showTitle && r.Title != "" {
		jr.Title = r.Title
	}
	if f.showHeaders && len(r.Headers) > 0 {
		jr.Headers = r.Headers
	}
	if r.FuzzData != nil && len(r.FuzzData.Fields) > 0 {
		jr.Fuzz = r.FuzzData.Fields
	}

	data, err := json.MarshalIndent(jr, "  ", "  ")
	if err != nil {
		return err
	}

	var prefix string
	if !f.hasPrinted {
		prefix = "[\n  "
		f.hasPrinted = true
	} else {
		prefix = ",\n  "
	}

	if _, err := io.WriteString(f.w, prefix); err != nil {
		return err
	}
	if _, err := f.w.Write(data); err != nil {
		return err
	}
	return nil
}

func (f *JSONFormatter) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true

	if !f.hasPrinted {
		_, err := io.WriteString(f.w, "[]\n")
		return err
	}

	_, err := io.WriteString(f.w, "\n]\n")
	return err
}
