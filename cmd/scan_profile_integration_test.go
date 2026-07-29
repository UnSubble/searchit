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

	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/targets"
)

func runScanProfileTest(args []string, hook func(config.Config)) error {
	cmd, opts := NewScanCmd()
	_ = opts
	_ = cmd
	opts.URL = "http://localhost" // Default target URL to satisfy validation

	// Reset silence flags that prior failing tests may have set.
	cmd.SilenceErrors = false
	cmd.SilenceUsage = false

	// Set test hook
	opts.testHookConfigApplied = hook
	defer func() { opts.testHookConfigApplied = nil }()

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
	// Use paranoid and lightspeed
	err := runScanProfileTest([]string{"scan", "--profile", "scan/paranoid", "--profile", "scan/lightspeed"}, func(cfg config.Config) {
		captured = cfg
	})
	if err != nil {
		t.Fatalf("scan command failed: %v", err)
	}

	// scan/lightspeed should win on timeout and threads
	if captured.Timeout != 3*time.Second {
		t.Errorf("expected timeout 3 (from lightspeed), got %v", captured.Timeout)
	}
	if captured.Threads != 256 {
		t.Errorf("expected threads 256 (from lightspeed), got %d", captured.Threads)
	}
	// But scan/paranoid's delay setting (since lightspeed doesn't define it) should still be 1s!
	if captured.Delay != time.Second {
		t.Errorf("expected delay 1s (retained from paranoid), got %v", captured.Delay)
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
	cmd, opts := NewScanCmd()
	_ = opts
	_ = cmd

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	opts.URL = "http://localhost"
	opts.URLFile = ""
	opts.Wordlist = ""
	opts.Threads = 32
	opts.Timeout = 10
	opts.Recursive = false
	opts.MaxDepth = 3
	opts.Strategy = "bfs"
	opts.ExcludeStatus = "404"
	opts.RecurseOn = "200,301,302,403"
	opts.NormalizePaths = false
	opts.CollapseSlashes = false
	opts.Output = ""
	opts.Format = "text"
	opts.Quiet = false
	opts.IncludeSize = ""
	opts.ExcludeSize = ""
	opts.IncludeHeaders = nil
	opts.ExcludeHeaders = nil
	opts.Delay = ""
	opts.Rate = 0
	opts.ConnectTimeout = "3s"
	opts.Profiles = []string{"scan/quick"}
	opts.resolvedTargets = []targets.Target{{URL: "http://localhost"}}

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
	cmd, opts := NewScanCmd()
	_ = opts
	_ = cmd

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	opts.URL = "http://localhost"
	opts.URLFile = ""
	opts.Wordlist = ""
	opts.Threads = 32
	opts.Timeout = 10
	opts.Recursive = false
	opts.MaxDepth = 3
	opts.Strategy = "bfs"
	opts.ExcludeStatus = "404"
	opts.RecurseOn = "200,301,302,403"
	opts.NormalizePaths = false
	opts.CollapseSlashes = false
	opts.Output = ""
	opts.Format = "json"
	opts.Quiet = false
	opts.IncludeSize = ""
	opts.ExcludeSize = ""
	opts.IncludeHeaders = nil
	opts.ExcludeHeaders = nil
	opts.Delay = ""
	opts.Rate = 0
	opts.ConnectTimeout = "3s"
	opts.Profiles = []string{"scan/quick"}
	opts.resolvedTargets = []targets.Target{{URL: "http://localhost"}}

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

// TestScanProfile_CLIOverridesPrecedence is a comprehensive regression test
// ensuring that an explicit CLI flag always wins over a conflicting value set
// by a loaded profile. It covers every flag listed in the bug report.
//
// Pipeline under test (must be respected in this exact order):
//
//	config.Default() → profile load → profile apply → applyCLIOverrides → engine
//
// scan/paranoid is used as the conflict profile because it sets the most
// fields: threads=1, timeout=20s, delay=1000ms, rate=1.0, random-agent=true.
func TestScanProfile_CLIOverridesPrecedence(t *testing.T) {
	type testCase struct {
		name  string
		args  []string
		check func(t *testing.T, cfg config.Config)
	}

	tests := []testCase{
		{
			// scan/paranoid sets threads: 1 — CLI -t 6 must win.
			name: "threads",
			args: []string{"scan", "--profile", "scan/paranoid", "-t", "6"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.Threads != 6 {
					t.Errorf("cfg.Threads = %d; want 6 (profile sets 1, CLI must win)", cfg.Threads)
				}
			},
		},
		{
			// scan/paranoid sets timeout: 20 — CLI --timeout 5 must win.
			name: "timeout",
			args: []string{"scan", "--profile", "scan/paranoid", "--timeout", "5"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.Timeout != 5*time.Second {
					t.Errorf("cfg.Timeout = %v; want 5s (profile sets 20s, CLI must win)", cfg.Timeout)
				}
			},
		},
		{
			// scan/paranoid sets delay: 1000ms — CLI --delay 500ms must win.
			name: "delay",
			args: []string{"scan", "--profile", "scan/paranoid", "--delay", "500ms"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.Delay != 500*time.Millisecond {
					t.Errorf("cfg.Delay = %v; want 500ms (profile sets 1000ms, CLI must win)", cfg.Delay)
				}
			},
		},
		{
			// scan/paranoid sets rate: 1.0 — CLI --rate 10 must win.
			name: "rate",
			args: []string{"scan", "--profile", "scan/paranoid", "--rate", "10"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.Rate != 10 {
					t.Errorf("cfg.Rate = %v; want 10 (profile sets 1.0, CLI must win)", cfg.Rate)
				}
			},
		},
		{
			// scan/base does not set recursive — CLI --recursive must set it to true.
			name: "recursive",
			args: []string{"scan", "--profile", "scan/base", "--recursive"},
			check: func(t *testing.T, cfg config.Config) {
				if !cfg.Recursive {
					t.Errorf("cfg.Recursive = false; want true (CLI --recursive must apply)")
				}
			},
		},
		{
			// scan/base does not set adaptive — CLI --adaptive must set it to true.
			name: "adaptive",
			args: []string{"scan", "--profile", "scan/base", "--adaptive"},
			check: func(t *testing.T, cfg config.Config) {
				if !cfg.Adaptive {
					t.Errorf("cfg.Adaptive = false; want true (CLI --adaptive must apply)")
				}
			},
		},
		{
			// scan/paranoid sets random-agent: true — CLI --random-agent=false must win.
			name: "random-agent false overrides profile true",
			args: []string{"scan", "--profile", "scan/paranoid", "--random-agent=false"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.RandomAgent {
					t.Errorf("cfg.RandomAgent = true; want false (profile sets true, CLI must override to false)")
				}
			},
		},
		{
			// No profile sets user-agent — CLI --user-agent must be applied.
			name: "user-agent",
			args: []string{"scan", "--profile", "scan/paranoid", "--user-agent", "TestBot/1.0"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.UserAgent != "TestBot/1.0" {
					t.Errorf("cfg.UserAgent = %q; want %q (CLI must apply)", cfg.UserAgent, "TestBot/1.0")
				}
			},
		},
		// Verify all three invocation forms produce the same effective thread count.
		// This is the exact scenario from the bug report.
		{
			name: "parity: no profile -t 6",
			args: []string{"scan", "-t", "6"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.Threads != 6 {
					t.Errorf("cfg.Threads = %d; want 6", cfg.Threads)
				}
			},
		},
		{
			name: "parity: scan/default -t 6",
			args: []string{"scan", "--profile", "scan/default", "-t", "6"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.Threads != 6 {
					t.Errorf("cfg.Threads = %d; want 6 with scan/default", cfg.Threads)
				}
			},
		},
		{
			name: "parity: scan/paranoid -t 6",
			args: []string{"scan", "--profile", "scan/paranoid", "-t", "6"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.Threads != 6 {
					t.Errorf("cfg.Threads = %d; want 6 (profile sets 1, CLI must win)", cfg.Threads)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured config.Config
			err := runScanProfileTest(tt.args, func(cfg config.Config) {
				captured = cfg
			})
			if err != nil {
				t.Fatalf("scan command failed: %v", err)
			}
			tt.check(t, captured)
		})
	}
}
