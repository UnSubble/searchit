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
		// --- New: empty extension sentinel tests ---
		{
			// Leading comma: explicit empty extension first, then php, html
			name:     "leading comma produces empty sentinel",
			input:    []string{",php,html"},
			expected: []string{"", "php", "html"},
		},
		{
			// Only a leading comma + one ext
			name:     "leading comma + single ext",
			input:    []string{",php"},
			expected: []string{"", "php"},
		},
		{
			// Multiple consecutive empty entries → only one "" due to deduplication
			name:     "multiple consecutive empty entries deduplicated",
			input:    []string{",,php,,html,"},
			expected: []string{"", "php", "html"},
		},
		{
			// Duplicate extensions including empty → deduplicated
			name:     "duplicates including empty deduplicated",
			input:    []string{",php,,html,php"},
			expected: []string{"", "php", "html"},
		},
		{
			// Ordering preserved: empty comes first when first comma is leading
			name:     "ordering preserved",
			input:    []string{",php,html"},
			expected: []string{"", "php", "html"},
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
			// No extensions: always return the base word alone.
			name:     "empty extensions list",
			baseWord: "admin",
			exts:     nil,
			expected: []string{"admin"},
		},
		{
			// Explicit extensions without "": only produce dotted variants.
			name:     "single extension no empty sentinel",
			baseWord: "admin",
			exts:     []string{"php"},
			expected: []string{"admin.php"},
		},
		{
			// Multiple extensions without "": only produce dotted variants.
			name:     "multiple extensions no empty sentinel",
			baseWord: "admin",
			exts:     []string{"php", "txt", "bak"},
			expected: []string{"admin.php", "admin.txt", "admin.bak"},
		},
		{
			// Empty sentinel at start → extensionless first, then dotted variants.
			name:     "empty sentinel + extensions",
			baseWord: "admin",
			exts:     []string{"", "php", "html"},
			expected: []string{"admin", "admin.php", "admin.html"},
		},
		{
			// Empty sentinel only → just the base word.
			name:     "empty sentinel only",
			baseWord: "admin",
			exts:     []string{""},
			expected: []string{"admin"},
		},
		{
			// Empty string never produces "admin." — the trailing dot is absent.
			name:     "empty extension does not produce trailing dot",
			baseWord: "test",
			exts:     []string{""},
			expected: []string{"test"},
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

// TestRoundTrip_ParseThenGenerateVariants verifies the complete CLI pipeline
// from Parse output into GenerateVariants — the critical integration invariant.
func TestRoundTrip_ParseThenGenerateVariants(t *testing.T) {
	tests := []struct {
		name     string
		cliExt   []string
		baseWord string
		expected []string
	}{
		{
			name:     "-e ,php,html generates extensionless + php + html",
			cliExt:   []string{",php,html"},
			baseWord: "test",
			expected: []string{"test", "test.php", "test.html"},
		},
		{
			name:     "-e php,html does NOT generate extensionless candidate",
			cliExt:   []string{"php,html"},
			baseWord: "test",
			expected: []string{"test.php", "test.html"},
		},
		{
			name:     "-e ,php generates both variants",
			cliExt:   []string{",php"},
			baseWord: "test",
			expected: []string{"test", "test.php"},
		},
		{
			name:     "-e ,,php,,html,php deduplicates empty and php",
			cliExt:   []string{",,php,,html,php"},
			baseWord: "test",
			expected: []string{"test", "test.php", "test.html"},
		},
		{
			name:     "no -e flag (nil) returns base word only",
			cliExt:   nil,
			baseWord: "test",
			expected: []string{"test"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exts, err := extensions.Parse(tc.cliExt)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			got := extensions.GenerateVariants(tc.baseWord, exts)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("got %v, want %v", got, tc.expected)
			}
		})
	}
}
