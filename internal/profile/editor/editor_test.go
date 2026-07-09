package editor_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/editor"
	_ "github.com/unsubble/searchit/internal/profile/scan"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	tempFile := args[len(args)-1]

	content := os.Getenv("MOCK_EDITOR_CONTENT")
	err := os.WriteFile(tempFile, []byte(content), 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write mock content: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func makeMockExecCommand(t *testing.T, mockContent string) func(name string, args ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		tempFile := args[len(args)-1]
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", tempFile)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"MOCK_EDITOR_CONTENT="+mockContent,
		)
		return cmd
	}
}

func TestFindEditor(t *testing.T) {
	// Backup env vars
	oldVisual := os.Getenv("VISUAL")
	oldEditor := os.Getenv("EDITOR")
	defer func() {
		os.Setenv("VISUAL", oldVisual)
		os.Setenv("EDITOR", oldEditor)
	}()

	t.Run("VISUAL precedence", func(t *testing.T) {
		os.Setenv("VISUAL", "myvisual --opt")
		os.Setenv("EDITOR", "myeditor")

		bin, args, err := editor.FindEditor()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bin != "myvisual" {
			t.Errorf("expected myvisual, got %q", bin)
		}
		if len(args) != 1 || args[0] != "--opt" {
			t.Errorf("expected args [--opt], got %v", args)
		}
	})

	t.Run("EDITOR fallback", func(t *testing.T) {
		os.Setenv("VISUAL", "")
		os.Setenv("EDITOR", "myeditor -f")

		bin, args, err := editor.FindEditor()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bin != "myeditor" {
			t.Errorf("expected myeditor, got %q", bin)
		}
		if len(args) != 1 || args[0] != "-f" {
			t.Errorf("expected args [-f], got %v", args)
		}
	})

	t.Run("default fallback error if not in PATH", func(t *testing.T) {
		os.Setenv("VISUAL", "")
		os.Setenv("EDITOR", "")
		oldPath := os.Getenv("PATH")
		os.Setenv("PATH", "")
		defer os.Setenv("PATH", oldPath)

		_, _, err := editor.FindEditor()
		if err == nil {
			t.Fatal("expected error when editor is not found in PATH, got nil")
		}
	})
}

func TestSession_EditUserProfile(t *testing.T) {
	_ = profile.RegisterBuiltinValidators()
	_ = profile.RegisterBuiltinDecoders()

	tmpDir := t.TempDir()
	store := &profile.DefaultStore{UserDir: tmpDir}

	validYAML := `schema: 1
name: scan/myuser
tool: scan
config:
  threads: 10
`
	scanDir := filepath.Join(tmpDir, "scan")
	_ = os.MkdirAll(scanDir, 0o755)
	_ = os.WriteFile(filepath.Join(scanDir, "myuser.yaml"), []byte(validYAML), 0o644)

	session := editor.NewSession(store)
	session.ExecCommand = makeMockExecCommand(t, `schema: 1
name: scan/myuser
tool: scan
config:
  threads: 55
`)

	err := session.Run("scan/myuser")
	if err != nil {
		t.Fatalf("unexpected error editing profile: %v", err)
	}

	p, err := store.Load("scan/myuser")
	if err != nil {
		t.Fatalf("failed to load edited profile: %v", err)
	}

	var cfg struct {
		Threads int `yaml:"threads"`
	}
	_ = p.Decode(&cfg)
	if cfg.Threads != 55 {
		t.Errorf("expected threads to be updated to 55, got %d", cfg.Threads)
	}
}

func TestSession_EditEmbeddedProfile(t *testing.T) {
	_ = profile.RegisterBuiltinValidators()
	_ = profile.RegisterBuiltinDecoders()

	tmpDir := t.TempDir()
	store := &profile.DefaultStore{UserDir: tmpDir}

	session := editor.NewSession(store)
	session.ExecCommand = makeMockExecCommand(t, `schema: 1
name: scan/quick
tool: scan
config:
  threads: 999
`)

	err := session.Run("scan/quick")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "scan", "quick.yaml")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected overridden profile file to be written to user directory: %v", err)
	}

	p, err := store.Load("scan/quick")
	if err != nil {
		t.Fatalf("failed to load overridden profile: %v", err)
	}

	var cfg struct {
		Threads int `yaml:"threads"`
	}
	_ = p.Decode(&cfg)
	if cfg.Threads != 999 {
		t.Errorf("expected threads to be 999, got %d", cfg.Threads)
	}
}

func TestSession_ValidationFailurePreservesTempFile(t *testing.T) {
	_ = profile.RegisterBuiltinValidators()
	_ = profile.RegisterBuiltinDecoders()

	tmpDir := t.TempDir()
	store := &profile.DefaultStore{UserDir: tmpDir}

	validYAML := `schema: 1
name: scan/myval
tool: scan
config:
  threads: 10
`
	scanDir := filepath.Join(tmpDir, "scan")
	_ = os.MkdirAll(scanDir, 0o755)
	profilePath := filepath.Join(scanDir, "myval.yaml")
	_ = os.WriteFile(profilePath, []byte(validYAML), 0o644)

	session := editor.NewSession(store)
	session.ExecCommand = makeMockExecCommand(t, `schema: 1
name: scan/myval
tool: scan
config:
  strategy: invalid
`)

	err := session.Run("scan/myval")
	if err == nil {
		t.Fatal("expected validation to fail, got nil")
	}

	valErr, ok := err.(*editor.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}

	if _, statErr := os.Stat(valErr.TempPath); statErr != nil {
		t.Fatalf("expected temporary file %q to be preserved: %v", valErr.TempPath, statErr)
	}
	defer os.Remove(valErr.TempPath)

	originalData, _ := os.ReadFile(profilePath)
	if !strings.Contains(string(originalData), "threads: 10") {
		t.Errorf("expected original file to remain untouched, but got:\n%s", string(originalData))
	}
}
