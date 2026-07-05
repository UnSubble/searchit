package targets_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unsubble/searchit/internal/targets"
)

func BenchmarkReadFile(b *testing.B) {
	tmpDir := b.TempDir()
	filePath := filepath.Join(tmpDir, "bench_targets.txt")

	content := "# comment line\nhttp://example.com\nhttp://abc.com\n  \n  http://def.com  \n"
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		b.Fatalf("failed to write test file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = targets.ReadFile(filePath)
	}
}
