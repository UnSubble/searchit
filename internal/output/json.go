package output

import (
	"encoding/json"
	"io"

	"github.com/unsubble/searchit/internal/engine"
)

type JSONFormatter struct {
	w       io.Writer
	results []jsonResult
}

type jsonResult struct {
	URL    string `json:"url"`
	Status int    `json:"status"`
	Length int64  `json:"length"`
	Depth  uint16 `json:"depth"`
}

func NewJSONFormatter(w io.Writer) *JSONFormatter {
	return &JSONFormatter{w: w}
}

func (f *JSONFormatter) Print(r engine.Result) error {
	f.results = append(f.results, jsonResult{
		URL:    r.URL,
		Status: r.StatusCode,
		Length: r.Length,
		Depth:  r.Depth,
	})
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
