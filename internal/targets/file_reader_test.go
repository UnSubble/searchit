package targets_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/unsubble/searchit/internal/targets"
)

func TestReadFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "targets.txt")

	content := `
# This is a comment
https://a.com

  https://b.com  
# Another comment

https://c.com
`
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	got, err := targets.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile unexpected error: %v", err)
	}

	want := []string{
		"https://a.com",
		"https://b.com",
		"https://c.com",
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadFile() = %v, want %v", got, want)
	}
}

func TestReadFile_MissingFile(t *testing.T) {
	_, err := targets.ReadFile("nonexistent_file.txt")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}
