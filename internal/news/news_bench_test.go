package news

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkFetch(b *testing.B) {
	tempDir := b.TempDir()
	originalNewsDir := newsDir
	newsDir = tempDir
	defer func() { newsDir = originalNewsDir }()

	err := os.WriteFile(filepath.Join(tempDir, "v1.0.0.md"), []byte("This is v1.0.0 news"), 0644)
	if err != nil {
		b.Fatalf("failed to create temp news file: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Fetch("v1.0.0")
	}
}

func BenchmarkFormatPreview(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		FormatPreview("v1.0.0", "TITLE", []string{"A"}, []string{"B"}, []string{"C"})
	}
}
