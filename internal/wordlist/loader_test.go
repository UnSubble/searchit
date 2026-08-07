package wordlist_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unsubble/searchit/internal/wordlist"
)

func TestLoadEffectiveWords_Embedded(t *testing.T) {
	ctx := context.Background()
	words, err := wordlist.LoadEffectiveWords(ctx, "")
	if err != nil {
		t.Fatalf("LoadEffectiveWords with empty path failed: %v", err)
	}

	if len(words) != 4751 {
		t.Errorf("expected 4751 entries in embedded wordlist, got %d", len(words))
	}

	// Verify no blank lines or comments
	for i, w := range words {
		if w == "" {
			t.Errorf("word at index %d is empty", i)
		}
		if w[0] == '#' {
			t.Errorf("word at index %d is a comment: %q", i, w)
		}
	}
}

func TestLoadEffectiveWords_ExplicitFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.txt")
	content := "admin\n# comment\napi\n\ntest\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	words, err := wordlist.LoadEffectiveWords(ctx, path)
	if err != nil {
		t.Fatalf("LoadEffectiveWords with file failed: %v", err)
	}

	expected := []string{"admin", "api", "test"}
	if len(words) != len(expected) {
		t.Fatalf("expected %d entries, got %d", len(expected), len(words))
	}

	for i, exp := range expected {
		if words[i] != exp {
			t.Errorf("word[%d] = %q; want %q", i, words[i], exp)
		}
	}
}

func TestLoadEffectiveWords_NonExistentFile(t *testing.T) {
	ctx := context.Background()
	_, err := wordlist.LoadEffectiveWords(ctx, "/path/does/not/exist.txt")
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}
