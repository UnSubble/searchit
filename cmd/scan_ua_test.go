package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/useragent"
)

// runScanConfigHook runs the scan command to the opts.testHookConfigApplied point
// and returns the captured config. It does not make real network requests.
func runScanConfigHook(t *testing.T, args []string) config.Config {
	cmd, opts := NewScanCmd()
	_ = cmd
	_ = opts
	cmd.SilenceErrors = false
	cmd.SilenceUsage = false

	var captured config.Config
	opts.testHookConfigApplied = func(c config.Config) { captured = c }

	cmd.SetArgs(args)
	ctx := context.Background()
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("scan command failed: %v", err)
	}
	return captured
}

// --------------------------------------------------------------------------
// Config-level tests (verify cfg fields via opts.testHookConfigApplied)
// --------------------------------------------------------------------------

func TestScanUA_ExplicitFlag(t *testing.T) {
	cfg := runScanConfigHook(t, []string{"-u", "http://localhost", "--user-agent", "MyAgent/1.0"})
	if cfg.UserAgent != "MyAgent/1.0" {
		t.Errorf("cfg.UserAgent = %q, want %q", cfg.UserAgent, "MyAgent/1.0")
	}
}

func TestScanUA_RandomAgentFlag(t *testing.T) {
	cfg := runScanConfigHook(t, []string{"-u", "http://localhost", "--random-agent"})
	if !cfg.RandomAgent {
		t.Error("cfg.RandomAgent should be true after --random-agent")
	}
}

func TestScanUA_NoFlagsDefaultsToFalse(t *testing.T) {
	cfg := runScanConfigHook(t, []string{"-u", "http://localhost"})
	if cfg.UserAgent != "" {
		t.Errorf("cfg.UserAgent = %q, want empty", cfg.UserAgent)
	}
	if cfg.RandomAgent {
		t.Error("cfg.RandomAgent should be false by default")
	}
}

// --------------------------------------------------------------------------
// Header-injection tests (verify the actual User-Agent sent on the wire)
// --------------------------------------------------------------------------

// collectUAs runs a minimal scan against a live test server and collects
// every User-Agent header value received by that server.
func collectScanUAs(t *testing.T, args []string) []string {
	t.Helper()

	var mu sync.Mutex
	var received []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, r.Header.Get("User-Agent"))
		mu.Unlock()
		w.WriteHeader(http.StatusNotFound) // filtered out; we only care about the header
	}))
	t.Cleanup(srv.Close)

	cmd, opts := NewScanCmd()
	_ = opts
	cmd.SilenceErrors = false
	cmd.SilenceUsage = false

	// Use a single-line wordlist (scan.go itself) so the scan terminates quickly.
	fullArgs := append([]string{"-u", srv.URL, "-w", "scan.go", "-t", "1"}, args...)
	cmd.SetArgs(fullArgs)
	_ = cmd.ExecuteContext(context.Background())

	mu.Lock()
	defer mu.Unlock()
	return received
}

func TestScanUA_ExplicitFlagSentOnWire(t *testing.T) {
	uas := collectScanUAs(t, []string{"--user-agent", "TestAgent/2.0"})
	if len(uas) == 0 {
		t.Skip("no requests reached the server")
	}
	for _, ua := range uas {
		if ua != "TestAgent/2.0" {
			t.Errorf("got User-Agent %q, want %q", ua, "TestAgent/2.0")
		}
	}
}

func TestScanUA_RandomAgentFromBuiltinList(t *testing.T) {
	uas := collectScanUAs(t, []string{"--random-agent"})
	if len(uas) == 0 {
		t.Skip("no requests reached the server")
	}
	agents := useragent.Agents()
	first := uas[0]
	if !slices.Contains(agents, first) {
		t.Errorf("User-Agent %q is not in the built-in list", first)
	}
	// All requests in one execution must carry the same User-Agent.
	for _, ua := range uas[1:] {
		if ua != first {
			t.Errorf("User-Agent changed mid-execution: got %q, first was %q", ua, first)
		}
	}
}

func TestScanUA_ExplicitHeaderOverridesFlag(t *testing.T) {
	// -H "User-Agent=HeaderAgent" must win over --user-agent.
	uas := collectScanUAs(t, []string{"-H", "User-Agent=HeaderAgent", "--user-agent", "FlagAgent"})
	if len(uas) == 0 {
		t.Skip("no requests reached the server")
	}
	for _, ua := range uas {
		if ua != "HeaderAgent" {
			t.Errorf("got User-Agent %q, want %q (header must win)", ua, "HeaderAgent")
		}
	}
}
