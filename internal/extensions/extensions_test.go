package extensions_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/unsubble/searchit/internal/extensions"
)

func TestParse_InlineAndMultiple(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "single extension",
			input:    []string{"php"},
			expected: []string{"php"},
		},
		{
			name:     "comma separated extensions",
			input:    []string{"php,txt,bak,json"},
			expected: []string{"php", "txt", "bak", "json"},
		},
		{
			name:     "multiple flags",
			input:    []string{"php", "txt", "bak"},
			expected: []string{"php", "txt", "bak"},
		},
		{
			name:     "leading dots and whitespace",
			input:    []string{" .php ", ".txt, bak ", ".json"},
			expected: []string{"php", "txt", "bak", "json"},
		},
		{
			name:     "deduplication preserving insertion order",
			input:    []string{"php", "txt", "php", "json", "txt"},
			expected: []string{"php", "txt", "json"},
		},
		{
			name:     "empty input",
			input:    nil,
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extensions.Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("got %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestParse_FileBased(t *testing.T) {
	tmpDir := t.TempDir()
	extFile := filepath.Join(tmpDir, "common.txt")
	content := "# Comment line\nphp\nhtml,htm\n.txt\n\n  # another comment\nbak\n"
	if err := os.WriteFile(extFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	got, err := extensions.Parse([]string{"@" + extFile, "php,json"})
	if err != nil {
		t.Fatalf("unexpected error parsing file ext: %v", err)
	}

	expected := []string{"php", "html", "htm", "txt", "bak", "json"}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("got %v, want %v", got, expected)
	}
}

func TestParse_FileNotFound(t *testing.T) {
	_, err := extensions.Parse([]string{"@nonexistent_file_xyz.txt"})
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestGenerateVariants(t *testing.T) {
	tests := []struct {
		name     string
		baseWord string
		exts     []string
		expected []string
	}{
		{
			name:     "empty extensions list",
			baseWord: "admin",
			exts:     nil,
			expected: []string{"admin"},
		},
		{
			name:     "single extension",
			baseWord: "admin",
			exts:     []string{"php"},
			expected: []string{"admin", "admin.php"},
		},
		{
			name:     "multiple extensions",
			baseWord: "admin",
			exts:     []string{"php", "txt", "bak"},
			expected: []string{"admin", "admin.php", "admin.txt", "admin.bak"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extensions.GenerateVariants(tc.baseWord, tc.exts)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("got %v, want %v", got, tc.expected)
			}
		})
	}
}
