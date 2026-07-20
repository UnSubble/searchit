package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/recursion"
)

func runScanProfileTest(args []string, hook func(config.Config)) error {
	resetFlagsForTest()
	flagURL = "http://localhost" // Default target URL to satisfy validation

	// Reset silence flags that prior failing tests may have set.
	scanCmd.SilenceErrors = false
	scanCmd.SilenceUsage = false

	// Set test hook
	testHookConfigApplied = hook
	defer func() { testHookConfigApplied = nil }()

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	scanCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.SetArgs(args)

	return cmd.ExecuteContext(context.Background())
}

func TestScanProfile_Defaults(t *testing.T) {
	var captured config.Config
	err := runScanProfileTest([]string{"scan"}, func(cfg config.Config) {
		captured = cfg
	})
	if err != nil {
		t.Fatalf("scan command failed: %v", err)
	}

	if captured.Threads != 32 {
		t.Errorf("expected default threads 32, got %d", captured.Threads)
	}
	if captured.Timeout != 10*time.Second {
		t.Errorf("expected default timeout 10, got %v", captured.Timeout)
	}
}

func TestScanProfile_SingleProfile(t *testing.T) {
	var captured config.Config
	err := runScanProfileTest([]string{"scan", "--profile", "scan/quick"}, func(cfg config.Config) {
		captured = cfg
	})
	if err != nil {
		t.Fatalf("scan command failed: %v", err)
	}

	if captured.Threads != 64 {
		t.Errorf("expected threads 64 from scan/quick, got %d", captured.Threads)
	}
	if captured.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5 from scan/quick, got %v", captured.Timeout)
	}
	// Verify that strategy remains default (BFS)
	if captured.Strategy != recursion.BFS {
		t.Errorf("expected default strategy BFS, got %v", captured.Strategy)
	}
}

func TestScanProfile_MultipleProfiles(t *testing.T) {
	var captured config.Config
	// scan/quick sets threads: 64, timeout: 5
	// scan/deep sets threads: 16, timeout: 30, recursive: true, max-depth: 5
	err := runScanProfileTest([]string{"scan", "--profile", "scan/quick", "--profile", "scan/deep"}, func(cfg config.Config) {
		captured = cfg
	})
	if err != nil {
		t.Fatalf("scan command failed: %v", err)
	}

	// scan/deep should win on threads and timeout, but quick's other defaults/profiles are applied
	if captured.Threads != 16 {
		t.Errorf("expected threads 16 (from deep overriding quick), got %d", captured.Threads)
	}
	if captured.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30 (from deep overriding quick), got %v", captured.Timeout)
	}
	if !captured.Recursive {
		t.Errorf("expected recursive true from deep")
	}
}

func TestScanProfile_OverlayOrdering(t *testing.T) {
	var captured config.Config
	// Reverse order of spring and node (which are independent targets resolving base first)
	err := runScanProfileTest([]string{"scan", "--profile", "scan-extra/node", "--profile", "scan-extra/spring"}, func(cfg config.Config) {
		captured = cfg
	})
	if err != nil {
		t.Fatalf("scan command failed: %v", err)
	}

	// scan-extra/spring should win on timeout
	if captured.Timeout != 12*time.Second {
		t.Errorf("expected timeout 12 (from spring overriding node), got %v", captured.Timeout)
	}
	// But scan-extra/node's threads setting (since scan-extra/spring doesn't define it) should still be 48!
	if captured.Threads != 48 {
		t.Errorf("expected threads 48 (retained from node because spring is an overlay and does not define it), got %d", captured.Threads)
	}
}

func TestScanProfile_CLIOverrides(t *testing.T) {
	var captured config.Config
	// CLI threads=8 should override scan/quick's threads=64
	err := runScanProfileTest([]string{"scan", "--profile", "scan/quick", "--threads", "8"}, func(cfg config.Config) {
		captured = cfg
	})
	if err != nil {
		t.Fatalf("scan command failed: %v", err)
	}

	if captured.Threads != 8 {
		t.Errorf("expected threads 8 (CLI override), got %d", captured.Threads)
	}
	// Timeout should still be 5 from the profile
	if captured.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5 (from profile), got %v", captured.Timeout)
	}
}

func TestScanProfile_UserProfile(t *testing.T) {
	// Create a temp directory for user profiles
	tmpDir := setupTestHome(t)
	scanDir := filepath.Join(tmpDir, "scan")
	if err := os.MkdirAll(scanDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	overrideYAML := `schema: 1
name: scan/custom
tool: scan
description: Custom user scan profile
config:
  threads: 99
  timeout: 45
`
	if err := os.WriteFile(filepath.Join(scanDir, "custom.yaml"), []byte(overrideYAML), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// We must temporarily override the home/user dir location or mock the store.
	// Since NewStore uses ~/.config/searchit/profiles by default, we can set Home directory env var
	// or override the default UserDir in loader.go.
	// Let's check how loader.go resolves UserDir:
	// func (s *DefaultStore) userDir() string {
	//     if s.UserDir != "" { return s.UserDir }
	//     home, _ := os.UserHomeDir()
	//     return filepath.Join(home, ".config", "searchit", "profiles")
	// }
	// In loader.go, s.UserDir can be set or it defaults to os.UserHomeDir().
	// In cmd/scan.go, it uses:
	// store := profile.NewStore()
	// which returns a DefaultStore. If we mock/override the environment variable HOME,
	// os.UserHomeDir() will return our temp dir!
	// Let's set HOME / USERPROFILE env var.

	// Since on some OS it's USERPROFILE or we want to construct the exact structure:
	// ~/.config/searchit/profiles/scan/custom.yaml
	userConfigDir := filepath.Join(tmpDir, ".config", "searchit", "profiles", "scan")
	if err := os.MkdirAll(userConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userConfigDir, "custom.yaml"), []byte(overrideYAML), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var captured config.Config
	err := runScanProfileTest([]string{"scan", "--profile", "scan/custom"}, func(cfg config.Config) {
		captured = cfg
	})
	if err != nil {
		t.Fatalf("scan command failed: %v", err)
	}

	if captured.Threads != 99 {
		t.Errorf("expected threads 99 (from user custom profile), got %d", captured.Threads)
	}
	if captured.Timeout != 45*time.Second {
		t.Errorf("expected timeout 45 (from user custom profile), got %v", captured.Timeout)
	}
}

func TestScanProfile_MissingProfile(t *testing.T) {
	err := runScanProfileTest([]string{"scan", "--profile", "scan/nonexistent"}, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent profile, got nil")
	}
	if !strings.Contains(err.Error(), "load failed") {
		t.Errorf("expected error message to indicate load failed, got: %v", err)
	}
}

func TestScanProfile_InvalidProfile(t *testing.T) {
	tmpDir := setupTestHome(t)

	userConfigDir := filepath.Join(tmpDir, ".config", "searchit", "profiles", "scan")
	_ = os.MkdirAll(userConfigDir, 0o755)

	// Malformed YAML
	malformed := `schema: 1
name: scan/malformed
tool: scan
config:
  threads: : invalid
`
	_ = os.WriteFile(filepath.Join(userConfigDir, "malformed.yaml"), []byte(malformed), 0o644)

	err := runScanProfileTest([]string{"scan", "--profile", "scan/malformed"}, nil)
	if err == nil {
		t.Fatal("expected error for malformed YAML profile, got nil")
	}
}

func TestScanProfile_ValidationFailure(t *testing.T) {
	tmpDir := setupTestHome(t)

	userConfigDir := filepath.Join(tmpDir, ".config", "searchit", "profiles", "scan")
	_ = os.MkdirAll(userConfigDir, 0o755)

	// Validation failure: threads must be at least 1
	invalidVal := `schema: 1
name: scan/invalidval
tool: scan
config:
  threads: 0
`
	_ = os.WriteFile(filepath.Join(userConfigDir, "invalidval.yaml"), []byte(invalidVal), 0o644)

	err := runScanProfileTest([]string{"scan", "--profile", "scan/invalidval"}, nil)
	if err == nil {
		t.Fatal("expected error for invalid validation, got nil")
	}
}

func TestScanProfile_DuplicateProfileLoading(t *testing.T) {
	var captured config.Config
	err := runScanProfileTest([]string{"scan", "--profile", "scan/quick", "--profile", "scan/quick"}, func(cfg config.Config) {
		captured = cfg
	})
	if err != nil {
		t.Fatalf("scan command failed: %v", err)
	}
	if captured.Threads != 64 {
		t.Errorf("expected threads 64, got %d", captured.Threads)
	}
}

func TestScanProfile_OutputText(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	flagURL = "http://localhost"
	flagURLFile = ""
	flagWordlist = ""
	flagThreads = 32
	flagTimeout = 10
	flagRecursive = false
	flagMaxDepth = 3
	flagStrategy = "bfs"
	flagExcludeStatus = "404"
	flagRecurseOn = "200,301,302,403"
	flagNormalizePaths = false
	flagCollapseSlashes = false
	flagOutput = ""
	flagFormat = "text"
	flagQuiet = false
	flagIncludeSize = ""
	flagExcludeSize = ""
	flagIncludeHeaders = nil
	flagExcludeHeaders = nil
	flagDelay = ""
	flagRate = 0
	flagConnectTimeout = "3s"
	flagProfiles = []string{"scan/quick"}
	resolvedTargets = []string{"http://localhost"}

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	scanCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })

	cmd.SetArgs([]string{"scan", "--profile", "scan/quick", "-u", "http://localhost"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = cmd.ExecuteContext(ctx)
	w.Close()

	var stdoutBuf bytes.Buffer
	_, _ = io.Copy(&stdoutBuf, r)
	out := stdoutBuf.String()

	if !strings.Contains(out, "[*] Profiles:") {
		t.Errorf("expected output to contain profiles header, got %q", out)
	}
	if !strings.Contains(out, "    scan/quick") {
		t.Errorf("expected output to list the profile name, got %q", out)
	}
}

func TestScanProfile_OutputJSON(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	flagURL = "http://localhost"
	flagURLFile = ""
	flagWordlist = ""
	flagThreads = 32
	flagTimeout = 10
	flagRecursive = false
	flagMaxDepth = 3
	flagStrategy = "bfs"
	flagExcludeStatus = "404"
	flagRecurseOn = "200,301,302,403"
	flagNormalizePaths = false
	flagCollapseSlashes = false
	flagOutput = ""
	flagFormat = "json"
	flagQuiet = false
	flagIncludeSize = ""
	flagExcludeSize = ""
	flagIncludeHeaders = nil
	flagExcludeHeaders = nil
	flagDelay = ""
	flagRate = 0
	flagConnectTimeout = "3s"
	flagProfiles = []string{"scan/quick"}
	resolvedTargets = []string{"http://localhost"}

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	scanCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })

	cmd.SetArgs([]string{"scan", "--profile", "scan/quick", "-u", "http://localhost", "--format", "json"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = cmd.ExecuteContext(ctx)
	w.Close()

	var stdoutBuf bytes.Buffer
	_, _ = io.Copy(&stdoutBuf, r)
	out := stdoutBuf.String()

	if strings.Contains(out, "[*] Profiles:") {
		t.Errorf("expected json output to NOT contain profiles header, got %q", out)
	}
}

func TestScanProfile_DependencyResolutionIntegration(t *testing.T) {
	tmpDir := setupTestHome(t)

	userConfigDir := filepath.Join(tmpDir, ".config", "searchit", "profiles", "scan")
	if err := os.MkdirAll(userConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	profileA := `schema: 1
name: scan/a
tool: scan
depends:
  - b
config:
  threads: 10
`
	profileB := `schema: 1
name: scan/b
tool: scan
depends:
  - c
config:
  threads: 20
  timeout: 5
`
	profileC := `schema: 1
name: scan/c
tool: scan
config:
  threads: 30
  timeout: 15
  recursive: true
`
	if err := os.WriteFile(filepath.Join(userConfigDir, "a.yaml"), []byte(profileA), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userConfigDir, "b.yaml"), []byte(profileB), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userConfigDir, "c.yaml"), []byte(profileC), 0o644); err != nil {
		t.Fatalf("write c: %v", err)
	}

	var captured config.Config
	err := runScanProfileTest([]string{"scan", "--profile", "scan/a"}, func(cfg config.Config) {
		captured = cfg
	})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if captured.Threads != 10 {
		t.Errorf("expected threads 10, got %d", captured.Threads)
	}
	if captured.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5, got %v", captured.Timeout)
	}
	if !captured.Recursive {
		t.Errorf("expected recursive true")
	}
}

func TestScanProfile_Newv050Profiles(t *testing.T) {
	t.Run("scan/default", func(t *testing.T) {
		var captured config.Config
		err := runScanProfileTest([]string{"scan", "--profile", "scan/default"}, func(cfg config.Config) {
			captured = cfg
		})
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if captured.Threads != 32 {
			t.Errorf("expected threads 32, got %d", captured.Threads)
		}
		if captured.Timeout != 10*time.Second {
			t.Errorf("expected timeout 10s, got %v", captured.Timeout)
		}
	})

	t.Run("scan/paranoid", func(t *testing.T) {
		var captured config.Config
		err := runScanProfileTest([]string{"scan", "--profile", "scan/paranoid"}, func(cfg config.Config) {
			captured = cfg
		})
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if captured.Threads != 1 {
			t.Errorf("expected threads 1, got %d", captured.Threads)
		}
		if captured.Delay != 1000*time.Millisecond {
			t.Errorf("expected delay 1000ms, got %v", captured.Delay)
		}
		if captured.Rate != 1.0 {
			t.Errorf("expected rate 1.0, got %v", captured.Rate)
		}
	})

	t.Run("scan/maniac", func(t *testing.T) {
		var captured config.Config
		err := runScanProfileTest([]string{"scan", "--profile", "scan/maniac"}, func(cfg config.Config) {
			captured = cfg
		})
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if captured.Threads != 128 {
			t.Errorf("expected threads 128, got %d", captured.Threads)
		}
		if !captured.Recursive {
			t.Errorf("expected recursive=true")
		}
		if captured.MaxDepth != 5 {
			t.Errorf("expected max-depth 5, got %d", captured.MaxDepth)
		}
	})

	t.Run("scan/lightspeed", func(t *testing.T) {
		var captured config.Config
		err := runScanProfileTest([]string{"scan", "--profile", "scan/lightspeed"}, func(cfg config.Config) {
			captured = cfg
		})
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if captured.Threads != 256 {
			t.Errorf("expected threads 256, got %d", captured.Threads)
		}
		if captured.Timeout != 3*time.Second {
			t.Errorf("expected timeout 3s, got %v", captured.Timeout)
		}
	})
}
