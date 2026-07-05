package targets

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzReadFile(f *testing.F) {
	f.Add([]byte("# comment\nhttp://example.com\n  http://abc.com\n"))
	f.Add([]byte(""))
	f.Add([]byte("\n\n\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "fuzz_targets.txt")

		if err := os.WriteFile(filePath, data, 0600); err != nil {
			t.Fatalf("failed to write fuzz file: %v", err)
		}

		_, _ = ReadFile(filePath)
	})
}
