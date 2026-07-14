package output_test

import (
	"bytes"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
)

func FuzzFormatters(f *testing.F) {
	f.Add("http://example.com/admin", 200, int64(1024), int(1), true)
	f.Add("", 404, int64(0), int(0), false)
	f.Add("invalid-url-$$$", 500, int64(-12), int(99), true)

	f.Fuzz(func(t *testing.T, url string, status int, length int64, depth int, accepted bool) {
		res := engine.Result{
			URL:        url,
			StatusCode: status,
			Length:     length,
			Depth:      uint16(depth),
			Accepted:   accepted,
		}

		// Text Formatter
		var textBuf bytes.Buffer
		tf := output.NewTextFormatter(&textBuf, false)
		_ = tf.Print(res)
		_ = tf.Close()

		// Quiet Text Formatter
		var quietBuf bytes.Buffer
		qtf := output.NewTextFormatter(&quietBuf, true)
		_ = qtf.Print(res)
		_ = qtf.Close()

		// JSON Formatter
		var jsonBuf bytes.Buffer
		jf := output.NewJSONFormatter(&jsonBuf)
		_ = jf.Print(res)
		_ = jf.Close()

		// NDJSON Formatter
		var ndjsonBuf bytes.Buffer
		ndf := output.NewNDJSONFormatter(&ndjsonBuf)
		_ = ndf.Print(res)
		_ = ndf.Close()

		// CSV Formatter
		var csvBuf bytes.Buffer
		cf := output.NewCSVFormatter(&csvBuf)
		_ = cf.Print(res)
		_ = cf.Close()

		// Markdown Formatter
		var mdBuf bytes.Buffer
		mdf := output.NewMarkdownFormatter(&mdBuf)
		_ = mdf.Print(res)
		_ = mdf.Close()
	})
}
