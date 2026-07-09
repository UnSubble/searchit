package editor

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// FindEditor looks up the preferred text editor from the environment (VISUAL, EDITOR)
// or falls back to platform defaults (nano for Linux/macOS, notepad.exe for Windows).
// Returns the editor binary, any leading arguments parsed from the env var, and an error.
func FindEditor() (string, []string, error) {
	if e := os.Getenv("VISUAL"); e != "" {
		bin, args := parseEditorCommand(e)
		if bin != "" {
			return bin, args, nil
		}
	}
	if e := os.Getenv("EDITOR"); e != "" {
		bin, args := parseEditorCommand(e)
		if bin != "" {
			return bin, args, nil
		}
	}

	var defaultEditor string
	if runtime.GOOS == "windows" {
		defaultEditor = "notepad.exe"
	} else {
		defaultEditor = "nano"
	}

	path, err := exec.LookPath(defaultEditor)
	if err != nil {
		return "", nil, fmt.Errorf("no editor found in VISUAL or EDITOR env vars, and default %q not found in PATH", defaultEditor)
	}
	return path, nil, nil
}

func parseEditorCommand(editorCmd string) (string, []string) {
	parts := strings.Fields(editorCmd)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}
