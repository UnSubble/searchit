package editor_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/editor"
	_ "github.com/unsubble/searchit/internal/profile/scan"
	"github.com/unsubble/searchit/internal/profile/types"
	"gopkg.in/yaml.v3"
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

func setupTestHome(t *testing.T) string {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")

	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)

	t.Cleanup(func() {
		os.Setenv("HOME", oldHome)
		os.Setenv("USERPROFILE", oldUserProfile)
	})
	return tmpDir
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

	// Test ValidationError formatting
	errStr := valErr.Error()
	if !strings.Contains(errStr, "Validation failed") || !strings.Contains(errStr, "Temporary file:") {
		t.Errorf("unexpected ValidationError string: %q", errStr)
	}
}

func TestSession_RunErrors(t *testing.T) {
	tmpDir := t.TempDir()
	store := &profile.DefaultStore{UserDir: tmpDir}

	session := editor.NewSession(store)

	t.Run("invalid profile name", func(t *testing.T) {
		err := session.Run("invalid_name")
		if err == nil {
			t.Fatal("expected error with invalid name format, got nil")
		}
	})

	t.Run("nonexistent profile", func(t *testing.T) {
		err := session.Run("scan/nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent profile, got nil")
		}
	})

	t.Run("editor run error", func(t *testing.T) {
		validYAML := `schema: 1
name: scan/myval
tool: scan
config:
  threads: 10
`
		scanDir := filepath.Join(tmpDir, "scan")
		_ = os.MkdirAll(scanDir, 0o755)
		_ = os.WriteFile(filepath.Join(scanDir, "myval.yaml"), []byte(validYAML), 0o644)

		session.ExecCommand = func(name string, args ...string) *exec.Cmd {
			// Construct a command that exits with error
			cmd := exec.Command("false")
			return cmd
		}

		err := session.Run("scan/myval")
		if err == nil {
			t.Fatal("expected editor failure, got nil")
		}
		if !strings.Contains(err.Error(), "editor failed to run") {
			t.Errorf("expected error message to contain 'editor failed to run', got: %v", err)
		}
	})
}

func TestSaveAtomically_Errors(t *testing.T) {
	t.Run("create destination directory failure", func(t *testing.T) {
		// Use a file as destination directory path to trigger MkdirAll error
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "blocking_file")
		_ = os.WriteFile(filePath, []byte("data"), 0o644)

		err := editor.SaveAtomically("temp.yaml", filepath.Join(filePath, "profile.yaml"))
		if err == nil {
			t.Fatal("expected MkdirAll error, got nil")
		}
	})

	t.Run("temp file read failure", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := editor.SaveAtomically(filepath.Join(tmpDir, "nonexistent.yaml"), filepath.Join(tmpDir, "out.yaml"))
		if err == nil {
			t.Fatal("expected read failure, got nil")
		}
	})
}

func TestSession_EditUserProfileYML(t *testing.T) {
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
	// Write to .yml extension
	_ = os.WriteFile(filepath.Join(scanDir, "myuser.yml"), []byte(validYAML), 0o644)

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

func TestSaveAtomically_CrossDevice(t *testing.T) {
	// Create temp file in /dev/shm (tmpfs)
	tempFile, err := os.CreateTemp("/dev/shm", "searchit-cross-*.yaml")
	if err != nil {
		t.Skip("skipping cross-device test: /dev/shm not writable or available")
		return
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	_, _ = tempFile.Write([]byte("cross-device-data"))
	tempFile.Close()

	// Destination in test temp directory (/home/... btrfs)
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "final.yaml")

	err = editor.SaveAtomically(tempPath, destPath)
	if err != nil {
		t.Fatalf("SaveAtomically failed on cross-device rename: %v", err)
	}

	// Verify target was correctly written
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}
	if string(data) != "cross-device-data" {
		t.Errorf("expected 'cross-device-data', got %q", string(data))
	}
}

func TestSaveAtomically_RenameFailure(t *testing.T) {
	tmpDir := t.TempDir()
	// create temp file
	tempFile := filepath.Join(tmpDir, "temp.yaml")
	_ = os.WriteFile(tempFile, []byte("data"), 0o644)

	// destPath is a directory, making direct rename and fallback copy atomic rename fail
	destPath := filepath.Join(tmpDir, "dest_dir")
	_ = os.MkdirAll(destPath, 0o755)

	err := editor.SaveAtomically(tempFile, destPath)
	if err == nil {
		t.Fatal("expected rename error, got nil")
	}
}

type mockStore struct {
	profiles map[string]*types.Profile
}

func (m *mockStore) Load(name string) (*types.Profile, error) {
	p, ok := m.profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile not found: %s", name)
	}
	return p, nil
}

func (m *mockStore) List() ([]profile.ProfileInfo, error) {
	return nil, nil
}

func (m *mockStore) LoadRaw(name string) ([]byte, error) {
	p, err := m.Load(name)
	if err != nil {
		return nil, err
	}
	var data bytes.Buffer
	enc := yaml.NewEncoder(&data)
	_ = enc.Encode(p)
	return data.Bytes(), nil
}

func (m *mockStore) Create(p types.Profile) error {
	return nil
}

func parseYAMLNode(yamlStr string) yaml.Node {
	var node yaml.Node
	_ = yaml.Unmarshal([]byte(yamlStr), &node)
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return *node.Content[0]
	}
	return node
}

func TestSession_NonDefaultStore(t *testing.T) {
	_ = profile.RegisterBuiltinValidators()
	_ = profile.RegisterBuiltinDecoders()

	tmpDir := setupTestHome(t)

	store := &mockStore{
		profiles: map[string]*types.Profile{
			"scan/myuser": {
				Schema: 1,
				Name:   "scan/myuser",
				Tool:   "scan",
				Config: parseYAMLNode("threads: 10"),
			},
		},
	}

	session := editor.NewSession(store)
	session.ExecCommand = makeMockExecCommand(t, `schema: 1
name: scan/myuser
tool: scan
config:
  threads: 88
`)

	err := session.Run("scan/myuser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	destPath := filepath.Join(tmpDir, ".config", "searchit", "profiles", "scan", "myuser.yaml")
	if _, err := os.Stat(destPath); err != nil {
		t.Fatalf("expected profile to be saved at %s, got error: %v", destPath, err)
	}
}

func TestFindEditor_DefaultSuccess(t *testing.T) {
	oldVisual := os.Getenv("VISUAL")
	oldEditor := os.Getenv("EDITOR")
	defer func() {
		os.Setenv("VISUAL", oldVisual)
		os.Setenv("EDITOR", oldEditor)
	}()

	t.Run("default success lookup", func(t *testing.T) {
		os.Setenv("VISUAL", "")
		os.Setenv("EDITOR", "")
		bin, _, err := editor.FindEditor()
		if err != nil {
			t.Logf("default editor lookup failed: %v", err)
		} else {
			if !strings.Contains(bin, "nano") && !strings.Contains(bin, "notepad") {
				t.Errorf("expected nano or notepad fallback, got %q", bin)
			}
		}
	})

	t.Run("spaces inside VISUAL and EDITOR env vars", func(t *testing.T) {
		os.Setenv("VISUAL", "   ")
		os.Setenv("EDITOR", "   ")
		bin, _, _ := editor.FindEditor()
		if bin == "" && os.Getenv("PATH") != "" {
			t.Errorf("expected fallback editor binary, got empty string")
		}
	})
}

func TestSession_DefaultStoreWithHome(t *testing.T) {
	_ = profile.RegisterBuiltinValidators()
	_ = profile.RegisterBuiltinDecoders()

	tmpDir := setupTestHome(t)

	store := &profile.DefaultStore{UserDir: ""}

	validYAML := `schema: 1
name: scan/myuser
tool: scan
config:
  threads: 10
`
	scanDir := filepath.Join(tmpDir, ".config", "searchit", "profiles", "scan")
	_ = os.MkdirAll(scanDir, 0o755)
	_ = os.WriteFile(filepath.Join(scanDir, "myuser.yaml"), []byte(validYAML), 0o644)

	session := editor.NewSession(store)
	session.ExecCommand = makeMockExecCommand(t, `schema: 1
name: scan/myuser
tool: scan
config:
  threads: 77
`)

	err := session.Run("scan/myuser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p, err := store.Load("scan/myuser")
	if err != nil {
		t.Fatalf("failed to load edited profile: %v", err)
	}

	var cfg struct {
		Threads int `yaml:"threads"`
	}
	_ = p.Decode(&cfg)
	if cfg.Threads != 77 {
		t.Errorf("expected threads to be updated to 77, got %d", cfg.Threads)
	}
}

func TestSession_RunUncoveredErrors(t *testing.T) {
	_ = profile.RegisterBuiltinValidators()
	_ = profile.RegisterBuiltinDecoders()

	t.Run("FindEditor failure inside session Run", func(t *testing.T) {
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
		_ = os.WriteFile(filepath.Join(scanDir, "myval.yaml"), []byte(validYAML), 0o644)

		oldVisual := os.Getenv("VISUAL")
		oldEditor := os.Getenv("EDITOR")
		oldPath := os.Getenv("PATH")
		os.Setenv("VISUAL", "")
		os.Setenv("EDITOR", "")
		os.Setenv("PATH", "") // clear PATH so LookPath fails
		defer func() {
			os.Setenv("VISUAL", oldVisual)
			os.Setenv("EDITOR", oldEditor)
			os.Setenv("PATH", oldPath)
		}()

		session := editor.NewSession(store)
		err := session.Run("scan/myval")
		if err == nil {
			t.Fatal("expected session Run to fail when no editor is found, got nil")
		}
	})

	t.Run("edited temp file deleted", func(t *testing.T) {
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
		_ = os.WriteFile(filepath.Join(scanDir, "myval.yaml"), []byte(validYAML), 0o644)

		session := editor.NewSession(store)
		session.ExecCommand = func(name string, args ...string) *exec.Cmd {
			tempFile := args[len(args)-1]
			_ = os.Remove(tempFile)
			cmd := exec.Command("true")
			return cmd
		}

		err := session.Run("scan/myval")
		if err == nil {
			t.Fatal("expected error when temp file is deleted by editor, got nil")
		}
		if !strings.Contains(err.Error(), "read edited temporary profile") {
			t.Errorf("expected error message to contain 'read edited temporary profile', got: %v", err)
		}
	})

	t.Run("decode profile error (malformed yaml)", func(t *testing.T) {
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
		_ = os.WriteFile(filepath.Join(scanDir, "myval.yaml"), []byte(validYAML), 0o644)

		session := editor.NewSession(store)
		session.ExecCommand = makeMockExecCommand(t, "invalid: [unclosed")

		err := session.Run("scan/myval")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		valErr, ok := err.(*editor.ValidationError)
		if !ok {
			t.Fatalf("expected ValidationError, got %T: %v", err, err)
		}
		if len(valErr.Errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(valErr.Errors))
		}
	})

	t.Run("generic validation failure (tool missing)", func(t *testing.T) {
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
		_ = os.WriteFile(filepath.Join(scanDir, "myval.yaml"), []byte(validYAML), 0o644)

		session := editor.NewSession(store)
		session.ExecCommand = makeMockExecCommand(t, `schema: 1
name: scan/myval
tool: ""
config:
  threads: 10
`)

		err := session.Run("scan/myval")
		if err == nil {
			t.Fatal("expected generic validation to fail, got nil")
		}
		valErr, ok := err.(*editor.ValidationError)
		if !ok {
			t.Fatalf("expected ValidationError, got %T: %v", err, err)
		}
		if len(valErr.Errors) != 1 || !strings.Contains(valErr.Errors[0].Error(), "profile tool is missing") {
			t.Errorf("expected tool missing error, got: %v", valErr.Errors)
		}
	})

	t.Run("SaveAtomically failure in session Run", func(t *testing.T) {
		tmpDir := t.TempDir()
		blockingFile := filepath.Join(tmpDir, "scan")
		_ = os.WriteFile(blockingFile, []byte("data"), 0o644)

		store := &profile.DefaultStore{UserDir: blockingFile}

		session := editor.NewSession(store)
		session.ExecCommand = makeMockExecCommand(t, `schema: 1
name: scan/myval
tool: scan
config:
  threads: 10
`)

		err := session.Run("scan/quick")
		if err == nil {
			t.Fatal("expected save atomically error, got nil")
		}
	})

	t.Run("CreateTempProfile failure inside session Run", func(t *testing.T) {
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
		_ = os.WriteFile(filepath.Join(scanDir, "myval.yaml"), []byte(validYAML), 0o644)

		oldTmpDir := os.Getenv("TMPDIR")
		oldTmp := os.Getenv("TMP")
		oldTemp := os.Getenv("TEMP")

		nonexistentDir := filepath.Join(tmpDir, "nonexistent-dir-12345")
		os.Setenv("TMPDIR", nonexistentDir)
		os.Setenv("TMP", nonexistentDir)
		os.Setenv("TEMP", nonexistentDir)

		defer func() {
			os.Setenv("TMPDIR", oldTmpDir)
			os.Setenv("TMP", oldTmp)
			os.Setenv("TEMP", oldTemp)
		}()

		session := editor.NewSession(store)
		session.ExecCommand = makeMockExecCommand(t, "data")
		err := session.Run("scan/myval")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestCreateTempProfile_Failure(t *testing.T) {
	oldTmpDir := os.Getenv("TMPDIR")
	oldTmp := os.Getenv("TMP")
	oldTemp := os.Getenv("TEMP")

	tmpDir := t.TempDir()
	nonexistentDir := filepath.Join(tmpDir, "nonexistent-directory-searchit-test")

	os.Setenv("TMPDIR", nonexistentDir)
	os.Setenv("TMP", nonexistentDir)
	os.Setenv("TEMP", nonexistentDir)

	defer func() {
		os.Setenv("TMPDIR", oldTmpDir)
		os.Setenv("TMP", oldTmp)
		os.Setenv("TEMP", oldTemp)
	}()

	_, err := editor.CreateTempProfile([]byte("data"))
	if err == nil {
		t.Fatal("expected error creating temporary profile when TMPDIR/TMP/TEMP is nonexistent, got nil")
	}
}
