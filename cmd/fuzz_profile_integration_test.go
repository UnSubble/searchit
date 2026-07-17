package cmd

import (
	"context"
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

	fuzzCmd.SilenceErrors = false
	fuzzCmd.SilenceUsage = false

	testHookConfigApplied = hook
	defer func() { testHookConfigApplied = nil }()

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	fuzzCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.SetArgs(args)

	return cmd.ExecuteContext(context.Background())
}

func TestFuzzProfile_Defaults(t *testing.T) {
	var captured config.Config
	err := runFuzzProfileTest([]string{"fuzz", "-w", "go.mod"}, func(cfg config.Config) {
		captured = cfg
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
	err := runFuzzProfileTest([]string{"fuzz", "--profile", "fuzz/login", "-w", "go.mod"}, func(cfg config.Config) {
		captured = cfg
	})
	if err != nil {
		t.Fatalf("fuzz command failed: %v", err)
	}

	if captured.Threads != 16 {
		t.Errorf("expected threads 16 (from fuzz/login), got %d", captured.Threads)
	}
	if captured.Timeout != 15*time.Second {
		t.Errorf("expected timeout 15s (from fuzz/login), got %v", captured.Timeout)
	}
	if captured.Method != "POST" {
		t.Errorf("expected method POST (from fuzz/login), got %q", captured.Method)
	}
}

func TestFuzzProfile_CLIOverrides(t *testing.T) {
	var captured config.Config
	err := runFuzzProfileTest([]string{"fuzz", "--profile", "fuzz/login", "-w", "go.mod", "--threads", "8", "-X", "PUT"}, func(cfg config.Config) {
		captured = cfg
	})
	if err != nil {
		t.Fatalf("fuzz command failed: %v", err)
	}

	if captured.Threads != 8 {
		t.Errorf("expected threads 8 (CLI override), got %d", captured.Threads)
	}
	if captured.Method != "PUT" {
		t.Errorf("expected method PUT (CLI override), got %q", captured.Method)
	}
	// Timeout should remain 15s from profile
	if captured.Timeout != 15*time.Second {
		t.Errorf("expected timeout 15s (from profile), got %v", captured.Timeout)
	}
}

func TestFuzzProfile_ExtraProfile(t *testing.T) {
	var captured config.Config
	err := runFuzzProfileTest([]string{"fuzz", "--profile", "fuzz-extra/graphql", "-w", "go.mod"}, func(cfg config.Config) {
		captured = cfg
	})
	if err != nil {
		t.Fatalf("fuzz command failed: %v", err)
	}

	if captured.Threads != 16 {
		t.Errorf("expected threads 16, got %d", captured.Threads)
	}
	if captured.Method != "POST" {
		t.Errorf("expected method POST, got %q", captured.Method)
	}
	// Check header from profile
	if len(captured.Headers) != 1 || captured.Headers[0] != "Content-Type=application/json" {
		t.Errorf("expected headers from profile [Content-Type=application/json], got %v", captured.Headers)
	}
}
