package env

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckMultipleInstallations(t *testing.T) {
	tempDir := t.TempDir()

	// Create directories for mock PATH
	bin1 := filepath.Join(tempDir, "bin1")
	bin2 := filepath.Join(tempDir, "bin2")
	bin3 := filepath.Join(tempDir, "bin3")
	os.MkdirAll(bin1, 0755)
	os.MkdirAll(bin2, 0755)
	os.MkdirAll(bin3, 0755)

	// Create real binary in bin1
	realExe := filepath.Join(bin1, "searchit")
	os.WriteFile(realExe, []byte("fake binary"), 0755)

	// Create symlink in bin2 pointing to bin1/searchit
	symExe := filepath.Join(bin2, "searchit")
	os.Symlink(realExe, symExe)

	// Create a completely distinct binary in bin3
	distinctExe := filepath.Join(bin3, "searchit")
	os.WriteFile(distinctExe, []byte("another fake binary"), 0755)

	// Set PATH to contain all three directories
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	t.Run("Duplicate path elimination via symlink canonicalization", func(t *testing.T) {
		os.Setenv("PATH", bin1+string(os.PathListSeparator)+bin2)

		res := CheckMultipleInstallations(nil)
		if res.HasMultiple {
			t.Errorf("Expected false for HasMultiple, got true. It should deduplicate symlinks.")
		}
		if len(res.UniquePaths) != 1 {
			t.Errorf("Expected exactly 1 unique path, got %d: %v", len(res.UniquePaths), res.UniquePaths)
		}
	})

	t.Run("True multiple installations", func(t *testing.T) {
		os.Setenv("PATH", bin1+string(os.PathListSeparator)+bin3)

		res := CheckMultipleInstallations(nil)
		if !res.HasMultiple {
			t.Errorf("Expected true for HasMultiple, got false. It should detect distinct installations.")
		}
		if len(res.UniquePaths) != 2 {
			t.Errorf("Expected exactly 2 unique paths, got %d: %v", len(res.UniquePaths), res.UniquePaths)
		}
	})

	t.Run("Mixed symlinks and multiple installations", func(t *testing.T) {
		os.Setenv("PATH", bin1+string(os.PathListSeparator)+bin2+string(os.PathListSeparator)+bin3)

		res := CheckMultipleInstallations(nil)
		if !res.HasMultiple {
			t.Errorf("Expected true for HasMultiple")
		}
		if len(res.UniquePaths) != 2 {
			t.Errorf("Expected exactly 2 unique paths (deduplicated symlink + distinct), got %d: %v", len(res.UniquePaths), res.UniquePaths)
		}
	})
}
