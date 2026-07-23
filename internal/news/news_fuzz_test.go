package news

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzFetch(f *testing.F) {
	tempDir := f.TempDir()
	originalNewsDir := newsDir
	newsDir = tempDir
	defer func() { newsDir = originalNewsDir }()

	f.Add("v1.0.0", "Some markdown content")
	f.Add("invalid", "")
	f.Add("v999.0.0", "GARBAGE DATA @#$%^&*()")

	f.Fuzz(func(t *testing.T, version string, fileContent string) {
		// Create file dynamically to test parser resilience
		os.WriteFile(filepath.Join(tempDir, version+".md"), []byte(fileContent), 0644)
		Fetch(version)
		os.Remove(filepath.Join(tempDir, version+".md"))
	})
}
