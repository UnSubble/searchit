package cmd

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/unsubble/searchit/internal/config"
)

func runFuzzProfileTest(args []string, hook func(config.Config)) error {
	// Reset all fuzz flag variables
	flagFuzzURL = "http://localhost/FUZZ"
	flagFuzzWordlist = ""
	flagFuzzFoo = ""
	flagFuzzBar = ""
	flagFuzzBuzz = ""
	flagFuzzMethod = "GET"
	flagFuzzData = ""
	flagFuzzHeaders = nil
	flagFuzzThreads = 32
	flagFuzzTimeout = 10
	flagFuzzExcludeStat = "404"
	flagFuzzIncSize = ""
	flagFuzzExcSize = ""
	flagFuzzOutput = ""
	flagFuzzFormat = "text"
	flagFuzzQuiet = false
	flagFuzzDelay = ""
	flagFuzzRate = 0
	flagFuzzCookie = ""
	flagFuzzProxy = ""
	flagFuzzMatchStatus = ""
	flagFuzzFilterStatus = ""
	flagFuzzMatchSize = ""
	flagFuzzFilterSize = ""
	flagFuzzMatchRegex = nil
	flagFuzzFilterRegex = nil
	flagFuzzMatchContent = nil
	flagFuzzFilterContent = nil
	flagFuzzShowHeaders = false
	flagFuzzShowTitle = false
	flagFuzzRequestFile = ""
	flagFuzzProfiles = nil
	flagFuzzStrategy = ""
	flagFuzzAdaptive = false

	fuzzCmd.SilenceErrors = false
	fuzzCmd.SilenceUsage = false

	testHookConfigApplied = hook
	defer func() { testHookConfigApplied = nil }()

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	fuzzCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
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
	// 1. No placeholders anywhere should fail
	err := runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/", "-w", "go.mod"}, func(cfg config.Config) {})
	if err == nil {
		t.Error("expected error for missing placeholders, got nil")
	}

	// 2. Placeholder in URL should succeed
	err = runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/FUZZ", "-w", "go.mod"}, func(cfg config.Config) {})
	if err != nil {
		t.Errorf("expected success for placeholder in URL, got %v", err)
	}

	// 3. Placeholder in Header should succeed
	err = runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/", "-H", "X-Header: FUZZ", "-w", "go.mod"}, func(cfg config.Config) {})
	if err != nil {
		t.Errorf("expected success for placeholder in Header, got %v", err)
	}

	// 4. Placeholder in Cookie should succeed
	err = runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/", "-b", "session=FUZZ", "-w", "go.mod"}, func(cfg config.Config) {})
	if err != nil {
		t.Errorf("expected success for placeholder in Cookie, got %v", err)
	}

	// 5. Placeholder in POST body should succeed
	err = runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/", "-X", "POST", "-d", "id=FUZZ", "-w", "go.mod"}, func(cfg config.Config) {})
	if err != nil {
		t.Errorf("expected success for placeholder in POST body, got %v", err)
	}

	// 6. Placeholder supplied entirely by a profile
	err = runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/", "--profile", "fuzz-extra/user-agent", "-w", "fuzz.go"}, func(cfg config.Config) {
		panic("STOP_EXECUTION")
	})
	if err != nil {
		t.Errorf("expected success for placeholder supplied by profile, got %v", err)
	}

	// 7. CLI overriding profile values (remove placeholder) should fail
	// fuzz-extra/user-agent adds User-Agent=FUZZ. If we override it with a CLI flag to something else, there is no placeholder.
	// Actually, wait: -H User-Agent=NotFuzz appends or replaces?
	// The profile merge process merges maps. If CLI has -H "User-Agent: NotFuzz", it overrides the profile's User-Agent.
	err = runFuzzProfileTest([]string{"fuzz", "-u", "http://localhost/", "--profile", "fuzz-extra/user-agent", "-H", "User-Agent: NotFuzz", "-w", "go.mod"}, func(cfg config.Config) {})
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
