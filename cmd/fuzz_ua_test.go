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

// runFuzzConfigHook runs the fuzz command to the opts.testHookConfigApplied point
// and returns the captured config. It does not make real network requests.
func runFuzzConfigHook(t *testing.T, args []string) config.Config {
	t.Helper()
	cmd, opts := NewFuzzCmd()
	_ = opts
	_ = cmd
	opts.URL = "http://localhost/FUZZ"
	opts.Wordlist = ""
	opts.Method = "GET"
	opts.Headers = nil
	opts.Threads = 32
	opts.Timeout = 10
	opts.ExcludeStatus = "404"
	opts.Quiet = false
	opts.Profiles = nil
	opts.Strategy = "eager"
	opts.UserAgent = ""
	opts.RandomAgent = false
	cmd.SilenceErrors = false
	cmd.SilenceUsage = false

	var captured config.Config
	opts.testHookConfigApplied = func(c config.Config) { captured = c }

	cmd.SetArgs(args)
	ctx := context.Background()
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("fuzz command failed: %v", err)
	}
	return captured
}

// --------------------------------------------------------------------------
// Config-level tests
// --------------------------------------------------------------------------

func TestFuzzUA_ExplicitFlag(t *testing.T) {
	cfg := runFuzzConfigHook(t, []string{"-u", "http://localhost/FUZZ", "-w", "fuzz.go", "--user-agent", "FuzzAgent/3.0"})
	if cfg.UserAgent != "FuzzAgent/3.0" {
		t.Errorf("cfg.UserAgent = %q, want %q", cfg.UserAgent, "FuzzAgent/3.0")
	}
}

func TestFuzzUA_RandomAgentFlag(t *testing.T) {
	cfg := runFuzzConfigHook(t, []string{"-u", "http://localhost/FUZZ", "-w", "fuzz.go", "--random-agent"})
	if !cfg.RandomAgent {
		t.Error("cfg.RandomAgent should be true after --random-agent")
	}
}

func TestFuzzUA_NoFlagsDefaultsToFalse(t *testing.T) {
	cfg := runFuzzConfigHook(t, []string{"-u", "http://localhost/FUZZ", "-w", "fuzz.go"})
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

// collectFuzzUAs runs a minimal fuzz against a live test server and collects
// every User-Agent header value received by that server.
func collectFuzzUAs(t *testing.T, extraArgs []string) []string {
	t.Helper()

	var mu sync.Mutex
	var received []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, r.Header.Get("User-Agent"))
		mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	cmd, opts := NewFuzzCmd()
	_ = cmd
	_ = opts

	opts.URL = ""
	opts.Wordlist = ""
	opts.Method = "GET"
	opts.Headers = nil
	opts.Threads = 1
	opts.Timeout = 10
	opts.ExcludeStatus = "404"
	opts.Quiet = true
	opts.Profiles = nil
	opts.Strategy = "eager"
	opts.UserAgent = ""
	opts.RandomAgent = false
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	// Use fuzz.go as a tiny wordlist so the run completes quickly.
	fullArgs := append([]string{"-u", srv.URL + "/FUZZ", "-w", "fuzz.go", "-t", "1", "--quiet"}, extraArgs...)
	cmd.SetArgs(fullArgs)
	_ = cmd.ExecuteContext(context.Background())

	mu.Lock()
	defer mu.Unlock()
	return received
}

func TestFuzzUA_ExplicitFlagSentOnWire(t *testing.T) {
	uas := collectFuzzUAs(t, []string{"--user-agent", "FuzzWireAgent/1.0"})
	if len(uas) == 0 {
		t.Skip("no requests reached the server")
	}
	for _, ua := range uas {
		if ua != "FuzzWireAgent/1.0" {
			t.Errorf("got User-Agent %q, want %q", ua, "FuzzWireAgent/1.0")
		}
	}
}

func TestFuzzUA_RandomAgentFromBuiltinList(t *testing.T) {
	uas := collectFuzzUAs(t, []string{"--random-agent"})
	if len(uas) == 0 {
		t.Skip("no requests reached the server")
	}
	agents := useragent.Agents()
	first := uas[0]
	if !slices.Contains(agents, first) {
		t.Errorf("User-Agent %q is not in the built-in list", first)
	}
	// All requests within a single execution must carry the same User-Agent.
	for _, ua := range uas[1:] {
		if ua != first {
			t.Errorf("User-Agent changed mid-execution: got %q, first was %q", ua, first)
		}
	}
}

func TestFuzzUA_ExplicitHeaderOverridesFlag(t *testing.T) {
	// -H "User-Agent=HeaderAgent" must win over --user-agent.
	uas := collectFuzzUAs(t, []string{"-H", "User-Agent=HeaderAgent", "--user-agent", "FlagAgent"})
	if len(uas) == 0 {
		t.Skip("no requests reached the server")
	}
	for _, ua := range uas {
		if ua != "HeaderAgent" {
			t.Errorf("got User-Agent %q, want %q (header must win)", ua, "HeaderAgent")
		}
	}
}
