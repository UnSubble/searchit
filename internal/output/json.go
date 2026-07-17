package output

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/unsubble/searchit/internal/engine"
)

type JSONFormatter struct {
	w           io.Writer
	results     []jsonResult
	showHeaders bool
	showTitle   bool
}

type jsonResult struct {
	URL     string      `json:"url"`
	Status  int         `json:"status"`
	Length  int64       `json:"length"`
	Depth   uint16      `json:"depth"`
	Title   string      `json:"title,omitempty"`
	Headers http.Header `json:"headers,omitempty"`
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
	f.results = append(f.results, jr)
	return nil
}

func (f *JSONFormatter) Close() error {
	if len(f.results) == 0 {
		_, err := io.WriteString(f.w, "[]\n")
		return err
	}
	data, err := json.MarshalIndent(f.results, "", "  ")
	if err != nil {
		return err
	}
	if _, err := f.w.Write(data); err != nil {
		return err
	}
	_, err = io.WriteString(f.w, "\n")
	return err
}
