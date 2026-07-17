package output

import (
	"encoding/json"
	"io"

	"github.com/unsubble/searchit/internal/engine"
)

type NDJSONFormatter struct {
	w           io.Writer
	showHeaders bool
	showTitle   bool
}

func NewNDJSONFormatter(w io.Writer, showHeaders bool, showTitle bool) *NDJSONFormatter {
	return &NDJSONFormatter{w: w, showHeaders: showHeaders, showTitle: showTitle}
}

func (f *NDJSONFormatter) Print(r engine.Result) error {
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
