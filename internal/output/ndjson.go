package output

import (
	"encoding/json"
	"io"

	"github.com/unsubble/searchit/internal/engine"
)

type NDJSONFormatter struct {
	w io.Writer
}

func NewNDJSONFormatter(w io.Writer) *NDJSONFormatter {
	return &NDJSONFormatter{w: w}
}

func (f *NDJSONFormatter) Print(r engine.Result) error {
	jr := jsonResult{
		URL:    r.URL,
		Status: r.StatusCode,
		Length: r.Length,
		Depth:  r.Depth,
	}
	data, err := json.Marshal(jr)
	if err != nil {
		return err
	}
	if _, err := f.w.Write(data); err != nil {
		return err
	}
	_, err = io.WriteString(f.w, "\n")
	return err
}

func (f *NDJSONFormatter) Close() error {
	return nil
}
