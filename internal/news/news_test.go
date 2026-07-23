package news

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetch(t *testing.T) {
	// Setup temporary NEWS directory for testing
	tempDir := t.TempDir()
	originalNewsDir := newsDir
	newsDir = tempDir
	defer func() { newsDir = originalNewsDir }()

	// Create a mock news file
	content := "This is v1.0.0 news."
	err := os.WriteFile(filepath.Join(tempDir, "v1.0.0.md"), []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create temp news file: %v", err)
	}

	tests := []struct {
		name    string
		version string
		wantErr bool
		want    string
	}{
		{"exists", "v1.0.0", false, content},
		{"does_not_exist", "v0.9.0", true, ""},
		{"malformed_version", "invalid", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Fetch(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("Fetch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.Content != tt.want {
				t.Errorf("Fetch() = %v, want %v", got.Content, tt.want)
			}
		})
	}
}

func TestFormatPreview(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		title    string
		new      []string
		improved []string
		fixed    []string
		contains []string
	}{
		{
			name:     "all_sections",
			version:  "v1.0.0",
			title:    "RELEASE v1.0.0",
			new:      []string{"Feature A"},
			improved: []string{"Speed"},
			fixed:    []string{"Bug 1"},
			contains: []string{"NEW", "IMPROVED", "FIXED", "Feature A", "Speed", "Bug 1"},
		},
		{
			name:     "only_new",
			version:  "v1.1.0",
			title:    "MINOR BUMP",
			new:      []string{"Feature B"},
			contains: []string{"NEW", "Feature B"},
		},
		{
			name:     "empty_sections",
			version:  "v1.0.1",
			title:    "PATCH",
			contains: []string{"VERSION", "TITLE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPreview(tt.version, tt.title, tt.new, tt.improved, tt.fixed)
			for _, c := range tt.contains {
				if !strings.Contains(got, c) {
					t.Errorf("FormatPreview() missing %s in %s", c, got)
				}
			}
		})
	}
}
