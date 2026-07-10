package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/pflag"
	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/editor"
)

var bootstrapOnce sync.Once

func runProfileCommand(args []string) (string, error) {
	bootstrapOnce.Do(func() {
		_ = profile.RegisterBuiltinValidators()
		_ = profile.RegisterBuiltinDecoders()
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
	invalidYAML := `schema: 1
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
	malformedYAML := `schema: 1
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

func TestHelperProcessCmd(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	tempFile := args[len(args)-1]

	content := os.Getenv("MOCK_EDITOR_CONTENT")
	_ = os.WriteFile(tempFile, []byte(content), 0o644)
	os.Exit(0)
}

func makeMockExecCommand(t *testing.T, mockContent string) func(name string, args ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		tempFile := args[len(args)-1]
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessCmd", "--", tempFile)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"MOCK_EDITOR_CONTENT="+mockContent,
		)
		return cmd
	}
}

func TestProfileEdit_Success(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create user profile
	_, err := runProfileCommand([]string{"profile", "create", "scan/myedit"})
	if err != nil {
		t.Fatalf("profile create failed: %v", err)
	}

	// Mock editor
	editor.DefaultExecCommand = makeMockExecCommand(t, `schema: 1
name: scan/myedit
tool: scan
config:
  threads: 42
`)
	defer func() {
		editor.DefaultExecCommand = exec.Command
	}()

	out, err := runProfileCommand([]string{"profile", "edit", "scan/myedit"})
	if err != nil {
		t.Fatalf("profile edit failed: %v", err)
	}

	if !strings.Contains(out, "Successfully edited profile: scan/myedit") {
		t.Errorf("expected success message, got %q", out)
	}

	// Verify content was updated via show command
	showOut, err := runProfileCommand([]string{"profile", "show", "scan/myedit"})
	if err != nil {
		t.Fatalf("profile show failed: %v", err)
	}
	if !strings.Contains(showOut, "threads: 42") {
		t.Errorf("expected updated threads config, got %q", showOut)
	}
}

func TestProfileEdit_ValidationFailure(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create user profile
	_, err := runProfileCommand([]string{"profile", "create", "scan/myeditval"})
	if err != nil {
		t.Fatalf("profile create failed: %v", err)
	}

	// Mock editor returning invalid overlay value (threads: 0)
	editor.DefaultExecCommand = makeMockExecCommand(t, `schema: 1
name: scan/myeditval
tool: scan
config:
  threads: 0
`)
	defer func() {
		editor.DefaultExecCommand = exec.Command
	}()

	out, err := runProfileCommand([]string{"profile", "edit", "scan/myeditval"})
	if err == nil {
		t.Fatal("expected profile edit to fail validation, but it succeeded")
	}

	if !strings.Contains(out, "Validation failed.") {
		t.Errorf("expected output to contain 'Validation failed.', got %q", out)
	}
	if !strings.Contains(out, "threads must be at least 1") {
		t.Errorf("expected validation message, got %q", out)
	}
	if !strings.Contains(out, "Temporary file:") {
		t.Errorf("expected temporary file path in output, got %q", out)
	}
}

func TestProfileEdit_Embedded(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Mock editor
	editor.DefaultExecCommand = makeMockExecCommand(t, `schema: 1
name: scan/quick
tool: scan
config:
  threads: 1000
`)
	defer func() {
		editor.DefaultExecCommand = exec.Command
	}()

	// Edit built-in 'scan/quick'
	_, err := runProfileCommand([]string{"profile", "edit", "scan/quick"})
	if err != nil {
		t.Fatalf("profile edit failed: %v", err)
	}

	// Verify user override created in user dir
	expectedPath := filepath.Join(tmpDir, ".config", "searchit", "profiles", "scan", "quick.yaml")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected user override file to be created: %v", err)
	}
}

func TestProfileCmd_MetadataShowAndList(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	userConfigDir := filepath.Join(tmpDir, ".config", "searchit", "profiles", "scan")
	_ = os.MkdirAll(userConfigDir, 0o755)

	profileYAML := `schema: 1
name: scan/wordpress
tool: scan
description: Wordpress scan
author: UnSubble
tags:
  - wordpress
  - php
homepage: https://github.com/unsubble/searchit
license: MIT
created: "2026-07-07"
updated: "2026-07-09"
depends:
  - base
experimental: false
config:
  threads: 32
`
	_ = os.WriteFile(filepath.Join(userConfigDir, "wordpress.yaml"), []byte(profileYAML), 0o644)

	outList, err := runProfileCommand([]string{"profile", "list"})
	if err != nil {
		t.Fatalf("profile list failed: %v", err)
	}
	if !strings.Contains(outList, "scan/wordpress (tags: wordpress, php)") {
		t.Errorf("expected list output to show tags, got:\n%s", outList)
	}

	outShow, err := runProfileCommand([]string{"profile", "show", "scan/wordpress"})
	if err != nil {
		t.Fatalf("profile show failed: %v", err)
	}
	expectedMetadataLines := []string{
		"author: UnSubble",
		"homepage: https://github.com/unsubble/searchit",
		"license: MIT",
		"created: \"2026-07-07\"",
		"updated: \"2026-07-09\"",
		"experimental: false",
	}
	for _, expected := range expectedMetadataLines {
		normExpected := strings.ReplaceAll(expected, `"`, "")
		normOutShow := strings.ReplaceAll(outShow, `"`, "")
		if !strings.Contains(normOutShow, normExpected) {
			t.Errorf("expected show output to contain metadata field %q, got:\n%s", expected, outShow)
		}
	}
	if !strings.Contains(outShow, "depends:") || !strings.Contains(outShow, "base") {
		t.Errorf("expected show output to contain depends list, got:\n%s", outShow)
	}
}
