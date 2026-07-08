package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/pflag"
	"github.com/unsubble/searchit/internal/profile"
)

var bootstrapOnce sync.Once

func runProfileCommand(args []string) (string, error) {
	bootstrapOnce.Do(func() {
		_ = profile.RegisterBuiltinValidators()
	})

	rootCmd.SetContext(nil)
	profileCmd.SetContext(nil)
	profileListCmd.SetContext(nil)
	profileShowCmd.SetContext(nil)

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	profileCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	profileListCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	profileShowCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.SetArgs(args)

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	ctx := context.Background()
	err := cmd.ExecuteContext(ctx)
	return buf.String(), err
}

func TestProfileListOutput(t *testing.T) {
	out, err := runProfileCommand([]string{"profile", "list"})
	if err != nil {
		t.Fatalf("profile list: %v", err)
	}

	if !strings.Contains(out, "BUILTIN") {
		t.Errorf("output missing BUILTIN header; got:\n%s", out)
	}

	wantProfiles := []string{"scan/base", "scan/quick", "scan/deep"}
	for _, want := range wantProfiles {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestProfileShowOutput(t *testing.T) {
	out, err := runProfileCommand([]string{"profile", "show", "scan/quick"})
	if err != nil {
		t.Fatalf("profile show scan/quick: %v", err)
	}

	// Should contain YAML content.
	wantFields := []string{"name:", "scan/quick", "tool:", "scan", "threads:"}
	for _, want := range wantFields {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestProfileShowMissing(t *testing.T) {
	_, err := runProfileCommand([]string{"profile", "show", "nonexistent"})
	if err == nil {
		t.Fatal("profile show nonexistent: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

func TestProfileShowAllEmbedded(t *testing.T) {
	names := []string{"scan/base", "scan/quick", "scan/deep"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			out, err := runProfileCommand([]string{"profile", "show", name})
			if err != nil {
				t.Fatalf("profile show %s: %v", name, err)
			}
			if !strings.Contains(out, "name: "+name) {
				t.Errorf("output missing 'name: %s'; got:\n%s", name, out)
			}
		})
	}
}

func TestProfileCreateOutput(t *testing.T) {
	// Set user config directory using tmpDir via HOME env override
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	out, err := runProfileCommand([]string{"profile", "create", "scan/mytest"})
	if err != nil {
		t.Fatalf("profile create scan/mytest failed: %v", err)
	}

	if !strings.Contains(out, "Created profile:") {
		t.Errorf("expected output to contain 'Created profile:', got:\n%s", out)
	}
	if !strings.Contains(out, "scan/mytest") {
		t.Errorf("expected output to contain profile name, got:\n%s", out)
	}
	if !strings.Contains(out, "Path:") {
		t.Errorf("expected output to contain Path:, got:\n%s", out)
	}

	// Verify file actually exists
	expectedPath := filepath.Join(tmpDir, ".config", "searchit", "profiles", "scan", "mytest.yaml")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("profile file not created at expected path: %v", err)
	}
}

func TestProfileCreate_Duplicate(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	_, err := runProfileCommand([]string{"profile", "create", "scan/dup"})
	if err != nil {
		t.Fatalf("first creation failed: %v", err)
	}

	// second creation should fail
	_, err = runProfileCommand([]string{"profile", "create", "scan/dup"})
	if err == nil {
		t.Fatal("expected duplicate creation to fail, but got no error")
	}
}

func TestProfileCreate_InvalidName(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	_, err := runProfileCommand([]string{"profile", "create", "invalidname"})
	if err == nil {
		t.Fatal("expected invalid name to fail, but got no error")
	}
}

func TestProfileValidate_Success(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create a valid user profile
	_, err := runProfileCommand([]string{"profile", "create", "scan/myval"})
	if err != nil {
		t.Fatalf("profile create failed: %v", err)
	}

	// Validate it
	out, err := runProfileCommand([]string{"profile", "validate", "scan/myval"})
	if err != nil {
		t.Fatalf("profile validate failed: %v", err)
	}

	if !strings.Contains(out, "Valid profile:") {
		t.Errorf("expected output to contain 'Valid profile:', got:\n%s", out)
	}
	if !strings.Contains(out, "scan/myval") {
		t.Errorf("expected output to contain profile name, got:\n%s", out)
	}
}

func TestProfileValidate_Failure_MissingFile(t *testing.T) {
	_, err := runProfileCommand([]string{"profile", "validate", "scan/missing"})
	if err == nil {
		t.Fatal("expected validation to fail for missing file, got nil")
	}
}

func TestProfileValidate_Failure_InvalidContent(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Write an invalid profile (e.g. invalid strategy)
	scanDir := filepath.Join(tmpDir, ".config", "searchit", "profiles", "scan")
	_ = os.MkdirAll(scanDir, 0o755)
	invalidYAML := `version: 1
name: scan/invalidval
tool: scan
config:
  strategy: invalid
`
	_ = os.WriteFile(filepath.Join(scanDir, "invalidval.yaml"), []byte(invalidYAML), 0o644)

	out, err := runProfileCommand([]string{"profile", "validate", "scan/invalidval"})
	if err == nil {
		t.Fatal("expected validation to fail for invalid strategy, got nil")
	}

	if !strings.Contains(out, "Validation failed:") {
		t.Errorf("expected output to contain 'Validation failed:', got:\n%s", out)
	}
	if !strings.Contains(out, "invalid strategy") {
		t.Errorf("expected output to contain 'invalid strategy' error, got:\n%s", out)
	}
}

func TestProfileValidate_Failure_MalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	scanDir := filepath.Join(tmpDir, ".config", "searchit", "profiles", "scan")
	_ = os.MkdirAll(scanDir, 0o755)
	malformedYAML := `version: 1
name: scan/malformed
tool: scan
config:
  threads: : invalid
`
	_ = os.WriteFile(filepath.Join(scanDir, "malformed.yaml"), []byte(malformedYAML), 0o644)

	_, err := runProfileCommand([]string{"profile", "validate", "scan/malformed"})
	if err == nil {
		t.Fatal("expected validation to fail for malformed YAML, got nil")
	}
}
