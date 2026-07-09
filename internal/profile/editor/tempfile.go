package editor

import (
	"fmt"
	"os"
	"path/filepath"
)

// CreateTempProfile creates a temporary file populated with the given YAML data.
// The file is created with a .yaml extension so editors can apply syntax highlighting.
func CreateTempProfile(data []byte) (string, error) {
	tmpFile, err := os.CreateTemp("", "searchit-profile-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create temporary profile file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(data); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("write temporary profile file: %w", err)
	}

	return tmpFile.Name(), nil
}

// SaveAtomically saves the contents of tempPath to destPath atomically.
// It tries to rename directly. If it fails (e.g. cross-device link), it falls back to
// copying to a temporary file in the destination directory and renaming.
func SaveAtomically(tempPath, destPath string) error {
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	// Try direct rename first (atomic if same filesystem)
	err := os.Rename(tempPath, destPath)
	if err == nil {
		return nil
	}

	// Fallback for cross-device rename
	data, err := os.ReadFile(tempPath)
	if err != nil {
		return fmt.Errorf("read temporary profile: %w", err)
	}

	// Use a .tmp suffix during writing so searchit loader ignores it
	tempDestFile, err := os.CreateTemp(destDir, "searchit-save-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary destination file: %w", err)
	}
	tempDestPath := tempDestFile.Name()
	defer func() {
		_ = os.Remove(tempDestPath)
	}()

	if _, err := tempDestFile.Write(data); err != nil {
		_ = tempDestFile.Close()
		return fmt.Errorf("write temporary destination file: %w", err)
	}
	if err := tempDestFile.Close(); err != nil {
		return fmt.Errorf("close temporary destination file: %w", err)
	}

	// Atomic rename to final destPath
	if err := os.Rename(tempDestPath, destPath); err != nil {
		return fmt.Errorf("atomic save rename: %w", err)
	}

	// Clean up original temp file
	_ = os.Remove(tempPath)
	return nil
}
