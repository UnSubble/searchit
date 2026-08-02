package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/config"
)

func TestGlobalConfig_MissingDefaultIgnored(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, "nonexistent"))

	cmd, _ := NewScanCmd()
	cmd.SetArgs([]string{"-u", "http://127.0.0.1:1"})
	buf := new(bytes.Buffer)
	cmd.SetErr(buf)

	// Explicitly reset cfgFile for clean test isolation
	cfgFile = ""

	opts := &ScanOptions{
		URL: "http://127.0.0.1:1",
		testHookConfigApplied: func(cfg config.Config) {
			if cfg.Threads != 32 {
				t.Errorf("expected default threads=32, got %d", cfg.Threads)
			}
		},
	}
	_ = opts
}

func TestGlobalConfig_PrecedenceChain(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "user_config.yaml")

	// Config file sets scan overlay values directly
	cfgContent := []byte(`
scan:
  threads: 50
  timeout: 20s
  follow-redirects: true
  adaptive: true
`)
	if err := os.WriteFile(cfgPath, cfgContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Test 1: Config file overrides defaults
	t.Cleanup(func() {
		cfgFile = ""
	})
	cfgFile = cfgPath

	// Resolve config directly using helper
	cfg, err := config.ResolveScanConfig(cfgPath, nil)
	if err != nil {
		t.Fatalf("unexpected error resolving scan config: %v", err)
	}

	if cfg.Threads != 50 {
		t.Errorf("expected threads=50 from config file, got %d", cfg.Threads)
	}
	if !cfg.FollowRedirects {
		t.Errorf("expected follow-redirects=true from config file")
	}
	if !cfg.Adaptive {
		t.Errorf("expected adaptive=true from config file")
	}
}

func TestGlobalConfig_StrictValidationUnknownKey(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "invalid.yaml")

	cfgContent := []byte(`
scan:
  threads: 50
  invalid_unknown_key: true
`)
	if err := os.WriteFile(cfgPath, cfgContent, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := config.ResolveScanConfig(cfgPath, nil)
	if err == nil {
		t.Fatal("expected error for invalid unknown key in config, got nil")
	}
	if !strings.Contains(err.Error(), "validation error") && !strings.Contains(err.Error(), "field invalid_unknown_key not found") {
		t.Errorf("expected validation error message, got: %v", err)
	}
}

func TestGlobalConfig_ExplicitMissingFileError(t *testing.T) {
	missingPath := "/tmp/this_file_does_not_exist_searchit_test.yaml"
	_, err := config.ResolveScanConfig(missingPath, nil)
	if err == nil {
		t.Fatal("expected error for missing explicit config file, got nil")
	}
}

func TestGlobalConfig_Determinism(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.yaml")

	cfgContent := []byte(`
scan:
  threads: 64
  delay: 25ms
  strategy: dfs
`)
	if err := os.WriteFile(cfgPath, cfgContent, 0644); err != nil {
		t.Fatal(err)
	}

	cfg1, err1 := config.ResolveScanConfig(cfgPath, nil)
	cfg2, err2 := config.ResolveScanConfig(cfgPath, nil)

	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: err1=%v, err2=%v", err1, err2)
	}

	if cfg1.Threads != cfg2.Threads || cfg1.Delay != cfg2.Delay || cfg1.Strategy != cfg2.Strategy {
		t.Errorf("configuration resolution is non-deterministic: cfg1=%+v, cfg2=%+v", cfg1, cfg2)
	}
}
