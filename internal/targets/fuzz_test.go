package targets

import (
	"bytes"
	"testing"
)

func FuzzReadFile(f *testing.F) {
	f.Add([]byte("# comment\nhttp://example.com\n  http://abc.com\n"))
	f.Add([]byte(""))
	f.Add([]byte("\n\n\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseTargets(bytes.NewReader(data))
	})
}
