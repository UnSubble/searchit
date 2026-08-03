package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/config"
)

func runFuzzProfileTest(args []string, hook func(config.Config)) error {
	cmd, opts := NewFuzzCmd()
	opts.testHookConfigApplied = hook
	_ = cmd
	_ = opts
	hasURLArg := false
	for _, a := range args {
		if a == "-u" || a == "--url" || a == "-r" || a == "--request" {
			hasURLArg = true
			break
		}
	}
	if !hasURLArg {
		opts.URL = "http://localhost/FUZZ"
	}
	opts.Wordlist = ""
	opts.Foo = ""
	opts.Bar = ""
	opts.Buzz = ""
	opts.Method = "GET"
	opts.Data = ""
	opts.Headers = nil
	opts.Threads = 32
	opts.Timeout = 10
	opts.ExcludeStatus = "404"
	opts.IncludeSize = ""
	opts.ExcludeSize = ""
	opts.Output = ""
	opts.Format = "text"
	opts.Quiet = false
	opts.Delay = ""
	opts.Rate = 0
	opts.Cookie = ""
	opts.Proxy = ""
	opts.MatchStatus = ""
	opts.FilterStatus = ""
	opts.MatchSize = ""
	opts.FilterSize = ""
	opts.MatchRegex = nil
	opts.FilterRegex = nil
	opts.MatchContent = nil
	opts.FilterContent = nil
	opts.ShowHeaders = false
	opts.ShowTitle = false
	opts.Request = ""
	opts.Profiles = nil
	opts.Strategy = ""
	opts.Adaptive = false

	cmd.SilenceErrors = false
	cmd.SilenceUsage = false

	for i, a := range args {
		if a == "go.mod" {
			if _, err := os.Stat("go.mod"); err != nil {
				args[i] = "../go.mod"
			}
		}
	}

	cmd.SetArgs(args)

	defer func() {
		if r := recover(); r != nil {
			if r != "STOP_EXECUTION" {
				panic(r)
			}
		}
	}()

	err := cmd.ExecuteContext(context.Background())
	if err != nil && err.Error() == "STOP_EXECUTION" {
		return nil
	}
	return err
}

func TestFuzzProfile_Defaults(t *testing.T) {
	var captured config.Config
	err := runFuzzProfileTest([]string{"fuzz", "-w", "go.mod"}, func(cfg config.Config) {
		captured = cfg
		panic("STOP_EXECUTION")
	})
	if err != nil {
		t.Fatalf("fuzz command failed: %v", err)
	}

	if captured.Threads != 32 {
		t.Errorf("expected default threads 32, got %d", captured.Threads)
	}
	if captured.Timeout != 10*time.Second {
		t.Errorf("expected default timeout 10s, got %v", captured.Timeout)
	}
}

func TestFuzzProfile_SingleProfile(t *testing.T) {
	var captured config.Config
	err := runFuzzProfileTest([]string{"fuzz", "--profile", "fuzz-extra/post-form", "--foo", "fuzz.go", "--bar", "fuzz.go", "-w", "fuzz.go"}, func(cfg config.Config) {
		captured = cfg
		panic("STOP_EXECUTION")
	})
	if err != nil {
		t.Fatalf("fuzz command failed: %v", err)
	}

	if captured.FuzzStrategy != "bfs" {
		t.Errorf("expected strategy bfs (from fuzz-extra/post-form), got %s", captured.FuzzStrategy)
	}
	if captured.Method != "POST" {
		t.Errorf("expected method POST (from fuzz-extra/post-form), got %q", captured.Method)
	}
}

func TestFuzzProfile_CLIOverrides(t *testing.T) {
	var captured config.Config
	err := runFuzzProfileTest([]string{"fuzz", "--profile", "fuzz-extra/post-form", "--foo", "fuzz.go", "--bar", "fuzz.go", "-w", "fuzz.go", "-s", "dfs", "-X", "PUT"}, func(cfg config.Config) {
		captured = cfg
		panic("STOP_EXECUTION")
	})
	if err != nil {
		t.Fatalf("fuzz command failed: %v", err)
	}

	if captured.FuzzStrategy != "dfs" {
		t.Errorf("expected strategy dfs (CLI override), got %s", captured.FuzzStrategy)
	}
	if captured.Method != "PUT" {
		t.Errorf("expected method PUT (CLI override), got %q", captured.Method)
	}
}

func TestFuzzProfile_ExtraProfile(t *testing.T) {
	var captured config.Config
	err := runFuzzProfileTest([]string{"fuzz", "--profile", "fuzz-extra/post-form", "--foo", "fuzz.go", "--bar", "fuzz.go", "-w", "fuzz.go"}, func(cfg config.Config) {
		captured = cfg
		panic("STOP_EXECUTION")
	})
	if err != nil {
		t.Fatalf("fuzz command failed: %v", err)
	}

	if captured.Method != "POST" {
		t.Errorf("expected method POST, got %q", captured.Method)
	}
	// Check header from profile
	if len(captured.Headers) != 1 || captured.Headers[0] != "Content-Type=application/x-www-form-urlencoded" {
		t.Errorf("expected headers from profile [Content-Type=application/x-www-form-urlencoded], got %v", captured.Headers)
	}
}

func TestFuzz_StrategyFlag(t *testing.T) {
	// 1. Default (no flag) should be "eager"
	var captured config.Config
	err := runFuzzProfileTest([]string{"fuzz", "-w", "fuzz.go"}, func(cfg config.Config) {
		captured = cfg
		panic("STOP_EXECUTION")
	})
	if err != nil {
		t.Fatalf("fuzz command failed: %v", err)
	}
	if captured.FuzzStrategy != "eager" {
		t.Errorf("expected default strategy eager, got %q", captured.FuzzStrategy)
	}

	// 2. Specific strategy (--strategy)
	for _, strategy := range []string{"eager", "bfs", "dfs"} {
		err = runFuzzProfileTest([]string{"fuzz", "-w", "go.mod", "--strategy", strategy}, func(cfg config.Config) {
			captured = cfg
			panic("STOP_EXECUTION")
		})
		if err != nil {
			t.Fatalf("fuzz command failed for strategy %s: %v", strategy, err)
		}
		if captured.FuzzStrategy != strategy {
			t.Errorf("expected strategy %s, got %q", strategy, captured.FuzzStrategy)
		}
	}

	// 2b. Specific strategy (-s shorthand)
	for _, strategy := range []string{"eager", "bfs", "dfs"} {
		err := runFuzzProfileTest([]string{"fuzz", "-s", strategy, "-w", "fuzz.go"}, func(cfg config.Config) {
			captured = cfg
			panic("STOP_EXECUTION")
		})
		if err != nil {
			t.Fatalf("fuzz command failed for strategy %s: %v", strategy, err)
		}
		if captured.FuzzStrategy != strategy {
			t.Errorf("expected strategy %s, got %q", strategy, captured.FuzzStrategy)
		}
	}

	// 3. Invalid strategy should return error
	err = runFuzzProfileTest([]string{"fuzz", "-w", "go.mod", "--strategy", "invalid"}, func(cfg config.Config) {
		captured = cfg
	})
	if err == nil {
		t.Error("expected error for invalid strategy, got nil")
	}

	// 4. Adaptive flag
	err = runFuzzProfileTest([]string{"fuzz", "-w", "go.mod", "--adaptive"}, func(cfg config.Config) {
		captured = cfg
	})
	if err != nil {
		t.Fatalf("fuzz command failed with --adaptive: %v", err)
	}
	if !captured.Adaptive {
		t.Error("expected adaptive true, got false")
	}
}

func TestFuzz_PlaceholderValidation(t *testing.T) {
	tmpWl := filepath.Join(t.TempDir(), "words.txt")
	_ = os.WriteFile(tmpWl, []byte("test\n"), 0644)

	// 1. No placeholders anywhere should fail
	err := runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/", "-w", tmpWl}, nil)
	if err == nil {
		t.Error("expected error for missing placeholders, got nil")
	}

	// 2. Placeholder in URL should succeed
	err = runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/FUZZ", "-w", tmpWl}, nil)
	if err != nil {
		t.Errorf("expected success for placeholder in URL, got %v", err)
	}

	// 3. Placeholder in Header should succeed
	err = runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/", "-H", "X-Header: FUZZ", "-w", tmpWl}, nil)
	if err != nil {
		t.Errorf("expected success for placeholder in Header, got %v", err)
	}

	// 4. Placeholder in Cookie should succeed
	err = runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/", "-b", "session=FUZZ", "-w", tmpWl}, nil)
	if err != nil {
		t.Errorf("expected success for placeholder in Cookie, got %v", err)
	}

	// 5. Placeholder in POST body should succeed
	err = runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/", "-X", "POST", "-d", "id=FUZZ", "-w", tmpWl}, nil)
	if err != nil {
		t.Errorf("expected success for placeholder in POST body, got %v", err)
	}

	// 6. Placeholder supplied entirely by a profile
	err = runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/", "--profile", "fuzz-extra/user-agent", "-w", tmpWl}, nil)
	if err != nil {
		t.Errorf("expected success for placeholder supplied by profile, got %v", err)
	}

	// 7. CLI overriding profile values (remove placeholder) should fail
	err = runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/", "--profile", "fuzz-extra/user-agent", "-H", "User-Agent: NotFuzz", "-w", tmpWl}, nil)
	if err == nil {
		t.Error("expected error when CLI overrides profile placeholder, got nil")
	}

	// 8. Placeholder supplied through profile inheritance
	// Create a temporary profile that inherits from fuzz-extra/user-agent
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	profileDir := tmpHome + "/.config/searchit/profiles/fuzz"
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatalf("failed to create profile dir: %v", err)
	}

	tmpProfile := `
schema: 1
name: fuzz/inheritance
tool: fuzz
depends:
  - fuzz-extra/user-agent
config: {}
`
	tmpFile := profileDir + "/inheritance.yaml"
	if err := os.WriteFile(tmpFile, []byte(tmpProfile), 0644); err != nil {
		t.Fatalf("failed to write tmp profile: %v", err)
	}

	err = runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/", "--profile", "fuzz/inheritance", "-w", "fuzz.go"}, func(cfg config.Config) {
		panic("STOP_EXECUTION")
	})
	if err != nil {
		t.Errorf("expected success for placeholder from inherited profile, got %v", err)
	}
}

// TestFuzzProfile_CLIOverridesPrecedence is the fuzz-side companion to
// TestScanProfile_CLIOverridesPrecedence. It verifies that every explicit CLI
// flag wins over a conflicting profile value in the fuzz pipeline.
//
// Pipeline under test:
//
//	config.Default() → profile load → profile apply → applyFuzzCLIOverrides → runner
//
// fuzz/paranoid is used as the conflict profile because it sets:
// threads=1, timeout=20s, delay=1000ms, rate=1, random-agent=true.
func TestFuzzProfile_CLIOverridesPrecedence(t *testing.T) {
	type testCase struct {
		name  string
		args  []string
		check func(t *testing.T, cfg config.Config)
	}

	tests := []testCase{
		{
			// fuzz/paranoid sets threads: 1 — CLI -t 6 must win.
			name: "threads",
			args: []string{"fuzz", "-u", "http://localhost/FUZZ", "-w", "fuzz.go", "--profile", "fuzz/paranoid", "-t", "6"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.Threads != 6 {
					t.Errorf("cfg.Threads = %d; want 6 (profile sets 1, CLI must win)", cfg.Threads)
				}
			},
		},
		{
			// fuzz/paranoid sets timeout: 20 — CLI --timeout 5 must win.
			name: "timeout",
			args: []string{"fuzz", "-u", "http://localhost/FUZZ", "-w", "fuzz.go", "--profile", "fuzz/paranoid", "--timeout", "5"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.Timeout != 5*time.Second {
					t.Errorf("cfg.Timeout = %v; want 5s (profile sets 20s, CLI must win)", cfg.Timeout)
				}
			},
		},
		{
			// fuzz/paranoid sets delay: 1000ms — CLI --delay 500ms must win.
			name: "delay",
			args: []string{"fuzz", "-u", "http://localhost/FUZZ", "-w", "fuzz.go", "--profile", "fuzz/paranoid", "--delay", "500ms"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.Delay != 500*time.Millisecond {
					t.Errorf("cfg.Delay = %v; want 500ms (profile sets 1000ms, CLI must win)", cfg.Delay)
				}
			},
		},
		{
			// fuzz/paranoid sets rate: 1 — CLI --rate 10 must win.
			name: "rate",
			args: []string{"fuzz", "-u", "http://localhost/FUZZ", "-w", "fuzz.go", "--profile", "fuzz/paranoid", "--rate", "10"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.Rate != 10 {
					t.Errorf("cfg.Rate = %v; want 10 (profile sets 1, CLI must win)", cfg.Rate)
				}
			},
		},
		{
			// fuzz/base does not set adaptive — CLI --adaptive must set it to true.
			name: "adaptive",
			args: []string{"fuzz", "-u", "http://localhost/FUZZ", "-w", "fuzz.go", "--profile", "fuzz/base", "--adaptive"},
			check: func(t *testing.T, cfg config.Config) {
				if !cfg.Adaptive {
					t.Errorf("cfg.Adaptive = false; want true (CLI --adaptive must apply)")
				}
			},
		},
		{
			// fuzz/paranoid sets random-agent: true — CLI --random-agent=false must win.
			name: "random-agent false overrides profile true",
			args: []string{"fuzz", "-u", "http://localhost/FUZZ", "-w", "fuzz.go", "--profile", "fuzz/paranoid", "--random-agent=false"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.RandomAgent {
					t.Errorf("cfg.RandomAgent = true; want false (profile sets true, CLI must override)")
				}
			},
		},
		{
			// No profile sets user-agent — CLI --user-agent must be applied.
			name: "user-agent",
			args: []string{"fuzz", "-u", "http://localhost/FUZZ", "-w", "fuzz.go", "--profile", "fuzz/paranoid", "--user-agent", "TestBot/1.0"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.UserAgent != "TestBot/1.0" {
					t.Errorf("cfg.UserAgent = %q; want %q (CLI must apply)", cfg.UserAgent, "TestBot/1.0")
				}
			},
		},
		// Verify all three invocation forms produce the same effective thread count.
		{
			name: "parity: no profile -t 6",
			args: []string{"fuzz", "-u", "http://localhost/FUZZ", "-w", "fuzz.go", "-t", "6"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.Threads != 6 {
					t.Errorf("cfg.Threads = %d; want 6", cfg.Threads)
				}
			},
		},
		{
			name: "parity: fuzz/default -t 6",
			args: []string{"fuzz", "-u", "http://localhost/FUZZ", "-w", "fuzz.go", "--profile", "fuzz/default", "-t", "6"},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.Threads != 6 {
					t.Errorf("cfg.Threads = %d; want 6 with fuzz/default", cfg.Threads)
				}
			},
		},
		{
			name: "parity: fuzz/paranoid -t 6",
			args: []string{"fuzz", "-u", "http://localhost/FUZZ", "-w", "fuzz.go", "--profile", "fuzz/paranoid", "-t", "6"},
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
			err := runFuzzProfileTest(tt.args, func(cfg config.Config) {
				captured = cfg
				panic("STOP_EXECUTION")
			})
			if err != nil {
				t.Fatalf("fuzz command failed: %v", err)
			}
			tt.check(t, captured)
		})
	}
}
