package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/config"
	"gopkg.in/yaml.v3"
)

func TestResolveConfigPath_CustomPath(t *testing.T) {
	custom := "/path/to/custom/config.yaml"
	path, isExplicit := config.ResolveConfigPath(custom)
	if path != custom || !isExplicit {
		t.Errorf("got (%q, %v), want (%q, true)", path, isExplicit, custom)
	}
}

func TestResolveConfigPath_XDG(t *testing.T) {
	tempDir := t.TempDir()
	xdgDir := filepath.Join(tempDir, "searchit")
	os.MkdirAll(xdgDir, 0755)
	cfgFile := filepath.Join(xdgDir, "config.yaml")
	os.WriteFile(cfgFile, []byte("scan:\n  threads: 64\n"), 0644)

	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", origXDG)
	os.Setenv("XDG_CONFIG_HOME", tempDir)

	path, isExplicit := config.ResolveConfigPath("")
	if path != cfgFile || isExplicit {
		t.Errorf("got (%q, %v), want (%q, false)", path, isExplicit, cfgFile)
	}
}

func TestLoadFile_ExplicitPath(t *testing.T) {
	tempDir := t.TempDir()
	cfgFile := filepath.Join(tempDir, "config.yaml")
	content := `
scan:
  threads: 128
  timeout: 5s
  adaptive: true
fuzz:
  threads: 64
  strategy: eager
`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	file, resolved, err := config.LoadFile(cfgFile)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	if resolved != cfgFile {
		t.Errorf("resolved = %q, want %q", resolved, cfgFile)
	}
	if file.Scan == nil || file.Scan.Threads == nil || *file.Scan.Threads != 128 {
		t.Errorf("unexpected scan threads: %+v", file.Scan)
	}
	if file.Fuzz == nil || file.Fuzz.Strategy == nil || *file.Fuzz.Strategy != "eager" {
		t.Errorf("unexpected fuzz strategy: %+v", file.Fuzz)
	}
}

func TestLoadFile_NonExistentExplicit(t *testing.T) {
	_, _, err := config.LoadFile("/nonexistent/searchit/config.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent explicit config file")
	}
}

func TestLoadFile_UnknownFieldsStrictRejection(t *testing.T) {
	tempDir := t.TempDir()
	cfgFile := filepath.Join(tempDir, "invalid.yaml")
	content := `
unknown_root_key: true
scan:
  threads: 64
`
	os.WriteFile(cfgFile, []byte(content), 0644)
	_, _, err := config.LoadFile(cfgFile)
	if err == nil {
		t.Fatal("expected error due to strict known fields validation")
	}
}

func TestScanOverlay_ApplyTo(t *testing.T) {
	raw := `
threads: 128
timeout: 15s
connect-timeout: 2s
delay: 100ms
rate: 25.5
wordlist: /path/to/dict.txt
ext: [php, html]
recursive: true
max-depth: 5
strategy: dfs
recurse-on: "200,301"
normalize-paths: true
collapse-slashes: true
format: json
quiet: true
follow-redirects: true
max-redirects: 10
insecure: true
exclude-status: "404,500"
match-status: "200"
include-size: "100-200"
exclude-size: "0"
match-regex: [".*admin.*"]
filter-regex: [".*logout.*"]
match-content: ["welcome"]
filter-content: ["error"]
headers: ["X-Custom: 123"]
cookies: "session=xyz"
include-headers: ["Server"]
exclude-headers: ["Date"]
show-headers: true
show-title: true
adaptive: true
random-agent: true
user-agent: "TestAgent/1.0"
proxy: "http://127.0.0.1:8080"
`
	var overlay config.ScanOverlay
	if err := yaml.Unmarshal([]byte(raw), &overlay); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}

	cfg := config.Default()
	config.ApplyScanOverlay(&cfg, overlay)

	if cfg.Threads != 128 {
		t.Errorf("Threads = %d, want 128", cfg.Threads)
	}
	if cfg.Timeout != 15*time.Second {
		t.Errorf("Timeout = %v, want 15s", cfg.Timeout)
	}
	if cfg.ConnectTimeout != 2*time.Second {
		t.Errorf("ConnectTimeout = %v, want 2s", cfg.ConnectTimeout)
	}
	if cfg.Delay != 100*time.Millisecond {
		t.Errorf("Delay = %v, want 100ms", cfg.Delay)
	}
	if cfg.Rate != 25.5 {
		t.Errorf("Rate = %v, want 25.5", cfg.Rate)
	}
	if cfg.Wordlist != "/path/to/dict.txt" {
		t.Errorf("Wordlist = %q", cfg.Wordlist)
	}
	if !cfg.Recursive || cfg.MaxDepth != 5 {
		t.Errorf("Recursive = %v, MaxDepth = %d", cfg.Recursive, cfg.MaxDepth)
	}
	if cfg.Proxy != "http://127.0.0.1:8080" {
		t.Errorf("Proxy = %q", cfg.Proxy)
	}
	if cfg.UserAgent != "TestAgent/1.0" {
		t.Errorf("UserAgent = %q", cfg.UserAgent)
	}
}

func TestFuzzOverlay_ApplyTo(t *testing.T) {
	raw := `
threads: 64
strategy: bfs
delay: 50ms
rate: 10
timeout: 8s
headers: ["Auth: Token"]
cookies: "user=admin"
proxy: "http://proxy.local:3128"
`
	var overlay config.FuzzOverlay
	if err := yaml.Unmarshal([]byte(raw), &overlay); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}

	if overlay.Threads == nil || *overlay.Threads != 64 {
		t.Errorf("unexpected Threads: %+v", overlay.Threads)
	}
	if overlay.Strategy == nil || *overlay.Strategy != "bfs" {
		t.Errorf("unexpected Strategy: %+v", overlay.Strategy)
	}
}
