package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigPath_Explicit(t *testing.T) {
	path, explicit := ResolveConfigPath("/tmp/custom.yaml")
	if path != "/tmp/custom.yaml" || !explicit {
		t.Fatalf("expected /tmp/custom.yaml explicit, got path=%q explicit=%v", path, explicit)
	}
}

func TestResolveConfigPath_XDG(t *testing.T) {
	tempDir := t.TempDir()
	xdgDir := filepath.Join(tempDir, "xdg")
	searchitDir := filepath.Join(xdgDir, "searchit")
	if err := os.MkdirAll(searchitDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(searchitDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("scan:\n  threads: 50\n"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	path, explicit := ResolveConfigPath("")
	if path != cfgPath || explicit {
		t.Fatalf("expected %q, got path=%q explicit=%v", cfgPath, path, explicit)
	}
}

func TestLoadFile_StrictUnknownKey(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "invalid.yaml")
	content := []byte(`
scan:
  threads: 64
  unknown_key_here: 123
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := LoadFile(cfgPath)
	if err == nil {
		t.Fatal("expected error for unknown configuration key, got nil")
	}
}

func TestResolveScanConfig_Precedence(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.yaml")
	content := []byte(`
scan:
  threads: 64
  timeout: 15s
  follow-redirects: true
  adaptive: true
  strategy: priority
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Resolve scan config with file only
	cfg, err := ResolveScanConfig(cfgPath, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Threads != 64 {
		t.Errorf("expected threads=64 from config file, got %d", cfg.Threads)
	}
	if !cfg.FollowRedirects {
		t.Errorf("expected follow-redirects=true from config scan section")
	}
	if !cfg.Adaptive {
		t.Errorf("expected adaptive=true from config scan section")
	}

	// 2. Add profile overlay (left to right)
	pThreads := 100
	p1 := ScanOverlay{
		Threads: &pThreads,
	}

	cfgWithProfile, err := ResolveScanConfig(cfgPath, []ScanOverlay{p1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfgWithProfile.Threads != 100 {
		t.Errorf("expected profile to override config file threads to 100, got %d", cfgWithProfile.Threads)
	}

	// 3. Ensure profiles in left-to-right order (p2 overrides p1)
	pThreads2 := 200
	p2 := ScanOverlay{
		Threads: &pThreads2,
	}
	cfgLeftRight, err := ResolveScanConfig(cfgPath, []ScanOverlay{p1, p2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfgLeftRight.Threads != 200 {
		t.Errorf("expected p2 (left-to-right) to override threads to 200, got %d", cfgLeftRight.Threads)
	}
}

func TestResolveFuzzConfig_Precedence(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "fuzz_config.yaml")
	content := []byte(`
fuzz:
  threads: 48
  strategy: bfs
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := ResolveFuzzConfig(cfgPath, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Threads != 48 {
		t.Errorf("expected threads=48 from config fuzz section, got %d", cfg.Threads)
	}
	if cfg.FuzzStrategy != "bfs" {
		t.Errorf("expected strategy=bfs from fuzz section, got %q", cfg.FuzzStrategy)
	}

	// Test profile override
	pStrat := "dfs"
	f1 := FuzzOverlay{
		Strategy: &pStrat,
	}
	cfgWithProfile, err := ResolveFuzzConfig(cfgPath, []FuzzOverlay{f1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfgWithProfile.FuzzStrategy != "dfs" {
		t.Errorf("expected profile to override strategy to dfs, got %q", cfgWithProfile.FuzzStrategy)
	}
}
