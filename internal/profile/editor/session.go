package editor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/unsubble/searchit/internal/profile"
)

// ValidationError represents a set of validation failures during editing.
type ValidationError struct {
	Errors   []error
	TempPath string
}

func (e *ValidationError) Error() string {
	var sb strings.Builder
	sb.WriteString("Validation failed.\n")
	for _, err := range e.Errors {
		sb.WriteString(fmt.Sprintf("- %v\n", err))
	}
	sb.WriteString(fmt.Sprintf("\nTemporary file:\n%s", e.TempPath))
	return sb.String()
}

// DefaultExecCommand is the command constructor used by Sessions.
// Overridden in tests to mock subprocesses.
var DefaultExecCommand = exec.Command

// Session manages the editing session of a profile.
type Session struct {
	Store       profile.Store
	ExecCommand func(name string, arg ...string) *exec.Cmd
}

// NewSession creates a new profile editing session.
func NewSession(store profile.Store) *Session {
	return &Session{
		Store:       store,
		ExecCommand: DefaultExecCommand,
	}
}

// Run executes the safe profile editing workflow.
func (s *Session) Run(name string) error {
	// 1. Validate profile name format
	if _, err := profile.ParseName(name); err != nil {
		return err
	}

	// 2. Load the raw profile YAML
	data, err := s.Store.LoadRaw(name)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}

	// 3. Create temporary profile file
	tempPath, err := CreateTempProfile(data)
	if err != nil {
		return err
	}

	// 4. Resolve editor
	bin, args, err := FindEditor()
	if err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	// 5. Append temp file path to arguments
	args = append(args, tempPath)

	// 6. Launch editor
	cmd := s.ExecCommand(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("editor failed to run: %w", err)
	}

	// 7. Read edited contents
	tempData, err := os.ReadFile(tempPath)
	if err != nil {
		return fmt.Errorf("read edited temporary profile: %w", err)
	}

	// 8. Validate new contents
	var validationErrors []error
	p, decodeErr := profile.DecodeProfile(tempData)
	if decodeErr != nil {
		validationErrors = append(validationErrors, decodeErr)
	} else {
		// Generic validation
		if err := profile.Validate(p); err != nil {
			validationErrors = append(validationErrors, err)
		}
		// Tool validator
		v := profile.GetValidator(p.Tool)
		if v != nil {
			if err := v.Validate(p); err != nil {
				validationErrors = append(validationErrors, err)
			}
		}
	}

	if len(validationErrors) > 0 {
		return &ValidationError{
			Errors:   validationErrors,
			TempPath: tempPath,
		}
	}

	// 9. Resolve final destination path
	var userDir string
	if ds, ok := s.Store.(*profile.DefaultStore); ok {
		if ds.UserDir != "" {
			userDir = ds.UserDir
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				_ = os.Remove(tempPath)
				return fmt.Errorf("resolve user home directory: %w", err)
			}
			userDir = filepath.Join(home, ".config", "searchit", "profiles")
		}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			_ = os.Remove(tempPath)
			return fmt.Errorf("resolve user home directory: %w", err)
		}
		userDir = filepath.Join(home, ".config", "searchit", "profiles")
	}

	destYAML := filepath.Join(userDir, name+".yaml")
	destYML := filepath.Join(userDir, name+".yml")

	var destPath string
	if _, err := os.Stat(destYML); err == nil {
		destPath = destYML
	} else {
		destPath = destYAML
	}

	// 10. Atomic save
	if err := SaveAtomically(tempPath, destPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	return nil
}
