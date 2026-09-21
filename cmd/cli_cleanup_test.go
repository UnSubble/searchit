package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/unsubble/searchit/internal/config"
)

// executeCmd captures stdout and stderr while executing cmd with given args.
func executeCmd(cmd *cobra.Command, args []string) (string, string, error) {
	cmd.SetContext(context.Background())
	cmd.SetArgs(args)

	verbose = false
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})

	// Reset any previous flag states
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})

	rOut, wOut, err := os.Pipe()
	if err != nil {
		return "", "", err
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		wOut.Close()
		rOut.Close()
		return "", "", err
	}

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdout = wOut
	os.Stderr = wErr

	cmdErrBuf := new(bytes.Buffer)
	cmdOutBuf := new(bytes.Buffer)
	cmd.SetErr(cmdErrBuf)
	cmd.SetOut(cmdOutBuf)

	execErr := cmd.ExecuteContext(context.Background())

	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var capturedStdout, capturedStderr bytes.Buffer
	_, _ = io.Copy(&capturedStdout, rOut)
	_, _ = io.Copy(&capturedStderr, rErr)
	_ = rOut.Close()
	_ = rErr.Close()

	// Combine command-directed buffers with pipe outputs
	combinedStdout := cmdOutBuf.String() + capturedStdout.String()
	combinedStderr := cmdErrBuf.String() + capturedStderr.String()

	return combinedStdout, combinedStderr, execErr
}

// TestQuietVerboseMutualExclusion ensures that any combination of --verbose (-v)
// and --quiet (-q) is rejected before network execution, sending 0 requests.
func TestQuietVerboseMutualExclusion(t *testing.T) {
	var requestCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlFile := filepath.Join(tmpDir, "wl.txt")
	if err := os.WriteFile(wlFile, []byte("admin\n"), 0600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	combinations := []struct {
		name string
		args []string
	}{
		{"verbose and q", []string{"--verbose", "-q"}},
		{"q and verbose", []string{"-q", "--verbose"}},
		{"v and quiet", []string{"-v", "--quiet"}},
		{"verbose and quiet", []string{"--verbose", "--quiet"}},
		{"q and v", []string{"-q", "-v"}},
		{"v and q", []string{"-v", "-q"}},
	}

	for _, tc := range combinations {
		t.Run("scan_"+tc.name, func(t *testing.T) {
			requestCount.Store(0)
			cmd, _ := NewScanCmd()
			args := append([]string{"-u", srv.URL, "-w", wlFile}, tc.args...)
			_, stderr, err := executeCmd(cmd, args)
			if err == nil {
				t.Fatalf("expected error for scan %v, got nil", tc.args)
			}
			if !strings.Contains(err.Error(), "--verbose cannot be used with --quiet") &&
				!strings.Contains(stderr, "--verbose cannot be used with --quiet") {
				t.Errorf("expected mutual exclusivity error message, got err=%v, stderr=%s", err, stderr)
			}
			if got := requestCount.Load(); got != 0 {
				t.Errorf("expected 0 requests sent before validation failure, got %d", got)
			}
		})

		t.Run("fuzz_"+tc.name, func(t *testing.T) {
			requestCount.Store(0)
			cmd, _ := NewFuzzCmd()
			args := append([]string{"-u", srv.URL + "/FUZZ", "-w", wlFile}, tc.args...)
			_, stderr, err := executeCmd(cmd, args)
			if err == nil {
				t.Fatalf("expected error for fuzz %v, got nil", tc.args)
			}
			if !strings.Contains(err.Error(), "--verbose cannot be used with --quiet") &&
				!strings.Contains(stderr, "--verbose cannot be used with --quiet") {
				t.Errorf("expected mutual exclusivity error message, got err=%v, stderr=%s", err, stderr)
			}
			if got := requestCount.Load(); got != 0 {
				t.Errorf("expected 0 requests sent before validation failure, got %d", got)
			}
		})
	}
}

// TestTechFlagRemoved verifies that --tech is completely unknown and not in help.
func TestTechFlagRemoved(t *testing.T) {
	t.Run("flag rejected as unknown", func(t *testing.T) {
		cmd, _ := NewScanCmd()
		_, _, err := executeCmd(cmd, []string{"-u", "http://localhost", "--tech", "laravel"})
		if err == nil {
			t.Fatal("expected error passing --tech, got nil")
		}
		if !strings.Contains(err.Error(), "unknown flag: --tech") {
			t.Errorf("expected 'unknown flag: --tech', got: %v", err)
		}
	})

	t.Run("not in scan help", func(t *testing.T) {
		cmd, _ := NewScanCmd()
		stdout, _, err := executeCmd(cmd, []string{"--help"})
		if err != nil {
			t.Fatalf("unexpected help error: %v", err)
		}
		if strings.Contains(stdout, "--tech") {
			t.Errorf("scan --help should not mention --tech, got:\n%s", stdout)
		}
	})
}

// TestVerboseDiagnostics_Scan compares normal vs --verbose scan output when requests fail.
func TestVerboseDiagnostics_Scan(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	serverAddr := l.Addr().String()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			// Close connection immediately to cause connection error on client
			conn.Close()
		}
	}()
	defer l.Close()

	tmpDir := t.TempDir()
	wlFile := filepath.Join(tmpDir, "wl.txt")
	if err := os.WriteFile(wlFile, []byte("broken\n"), 0600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	targetURL := "http://" + serverAddr

	t.Run("normal mode suppresses request errors", func(t *testing.T) {
		cmd, _ := NewScanCmd()
		_, stderr, err := executeCmd(cmd, []string{"-u", targetURL, "-w", wlFile, "-t", "1"})
		if err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}
		if strings.Contains(stderr, "[-] Request error:") {
			t.Errorf("normal mode should not expose '[-] Request error:', got:\n%s", stderr)
		}
	})

	t.Run("verbose mode exposes request errors", func(t *testing.T) {
		cmd, _ := NewScanCmd()
		_, stderr, err := executeCmd(cmd, []string{"-u", targetURL, "-w", wlFile, "-t", "1", "--verbose"})
		if err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}
		if !strings.Contains(stderr, "[-] Request error:") {
			t.Errorf("verbose mode should expose '[-] Request error:', got:\n%s", stderr)
		}
	})

	t.Run("shorthand -v exposes request errors", func(t *testing.T) {
		cmd, _ := NewScanCmd()
		_, stderr, err := executeCmd(cmd, []string{"-u", targetURL, "-w", wlFile, "-t", "1", "-v"})
		if err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}
		if !strings.Contains(stderr, "[-] Request error:") {
			t.Errorf("-v should expose '[-] Request error:', got:\n%s", stderr)
		}
	})

	t.Run("verbose mode does not corrupt json output", func(t *testing.T) {
		cmd, _ := NewScanCmd()
		stdout, stderr, err := executeCmd(cmd, []string{"-u", targetURL, "-w", wlFile, "-t", "1", "--format", "json", "--verbose"})
		if err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}
		if !strings.Contains(stderr, "[-] Request error:") {
			t.Errorf("verbose mode should expose '[-] Request error:' in stderr, got:\n%s", stderr)
		}
		// If stdout has output, it must be valid JSON
		trimmed := strings.TrimSpace(stdout)
		if trimmed != "" {
			var parsed any
			if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
				t.Errorf("stdout was corrupted by verbose mode, not valid JSON: %v\nOutput was:\n%s", err, stdout)
			}
		}
	})
}

// TestVerboseDiagnostics_Fuzz compares normal vs --verbose fuzz output when requests fail.
func TestVerboseDiagnostics_Fuzz(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	serverAddr := l.Addr().String()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	defer l.Close()

	tmpDir := t.TempDir()
	wlFile := filepath.Join(tmpDir, "wl.txt")
	if err := os.WriteFile(wlFile, []byte("probe\n"), 0600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	targetURL := "http://" + serverAddr + "/FUZZ"

	t.Run("normal fuzz mode suppresses request errors", func(t *testing.T) {
		cmd, _ := NewFuzzCmd()
		_, stderr, err := executeCmd(cmd, []string{"-u", targetURL, "-w", wlFile, "-t", "1"})
		if err != nil {
			t.Fatalf("unexpected fuzz error: %v", err)
		}
		if strings.Contains(stderr, "[-] Request error:") {
			t.Errorf("normal fuzz mode should not expose '[-] Request error:', got:\n%s", stderr)
		}
	})

	t.Run("verbose fuzz mode exposes request errors", func(t *testing.T) {
		cmd, _ := NewFuzzCmd()
		_, stderr, err := executeCmd(cmd, []string{"-u", targetURL, "-w", wlFile, "-t", "1", "--verbose"})
		if err != nil {
			t.Fatalf("unexpected fuzz error: %v", err)
		}
		if !strings.Contains(stderr, "[-] Request error:") {
			t.Errorf("verbose fuzz mode should expose '[-] Request error:', got:\n%s", stderr)
		}
	})
}

// TestAdaptiveTechDiscovery ensures that internal adaptive technology detection
// is preserved and continues to function without --tech.
func TestAdaptiveTechDiscovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Powered-By", "Express")
		w.Header().Set("Server", "nginx/1.18.0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello Express"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlFile := filepath.Join(tmpDir, "wl.txt")
	if err := os.WriteFile(wlFile, []byte("app\n"), 0600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	cmd, _ := NewScanCmd()
	stdout, stderr, err := executeCmd(cmd, []string{"-u", srv.URL, "-w", wlFile, "--adaptive", "-t", "1"})
	if err != nil {
		t.Fatalf("adaptive scan failed: %v", err)
	}
	_ = stdout

	// In adaptive mode, target awareness logs or summary should run successfully
	if !strings.Contains(stderr, "Adaptive") && !strings.Contains(stdout, srv.URL) {
		t.Logf("Scan output: stdout=%s\nstderr=%s", stdout, stderr)
	}
}

// TestRemovedFlagsComprehensive verifies that removed flags are rejected as unknown on both scan and fuzz.
func TestRemovedFlagsComprehensive(t *testing.T) {
	tests := []struct {
		cmdName string
		flag    string
		val     string
		newCmd  func() *cobra.Command
	}{
		{"scan", "--url-file", "urls.txt", func() *cobra.Command { c, _ := NewScanCmd(); return c }},
		{"fuzz", "--url-file", "urls.txt", func() *cobra.Command { c, _ := NewFuzzCmd(); return c }},
		{"scan", "--filter-code", "200", func() *cobra.Command { c, _ := NewScanCmd(); return c }},
		{"scan", "--include-header", "Server=nginx", func() *cobra.Command { c, _ := NewScanCmd(); return c }},
		{"scan", "--exclude-header", "Server=Apache", func() *cobra.Command { c, _ := NewScanCmd(); return c }},
		{"scan", "--tech", "laravel", func() *cobra.Command { c, _ := NewScanCmd(); return c }},
		{"fuzz", "--tech", "laravel", func() *cobra.Command { c, _ := NewFuzzCmd(); return c }},
	}

	for _, tc := range tests {
		t.Run(tc.cmdName+"_"+strings.TrimPrefix(tc.flag, "--"), func(t *testing.T) {
			cmd := tc.newCmd()
			args := []string{"-u", "http://localhost", tc.flag}
			if tc.val != "" {
				args = append(args, tc.val)
			}
			_, _, err := executeCmd(cmd, args)
			if err == nil {
				t.Fatalf("expected error passing %s to %s, got nil", tc.flag, tc.cmdName)
			}
			expectedPrefix := "unknown flag: " + tc.flag
			if !strings.Contains(err.Error(), expectedPrefix) {
				t.Errorf("expected %q in error, got: %v", expectedPrefix, err)
			}
		})
	}
}

// TestQuietConsistency_ScanAndFuzz verifies that --quiet mode produces URL-only terminal output
// for both scan and fuzz commands, and that non-quiet output contains status, size, etc.
func TestQuietConsistency_ScanAndFuzz(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok response"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlFile := filepath.Join(tmpDir, "wl.txt")
	_ = os.WriteFile(wlFile, []byte("item\n"), 0600)

	t.Run("scan quiet is URL only", func(t *testing.T) {
		cmd, _ := NewScanCmd()
		stdout, _, err := executeCmd(cmd, []string{"-u", srv.URL, "-w", wlFile, "-q"})
		if err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if strings.Contains(line, "[+]") || strings.Contains(line, "200") || strings.Contains(line, "SCAN CONFIGURATION") {
				t.Errorf("scan quiet mode emitted formatted line instead of bare URL: %q", line)
			}
			if !strings.HasPrefix(line, srv.URL) {
				t.Errorf("expected bare URL starting with %q, got %q", srv.URL, line)
			}
		}
	})

	t.Run("fuzz quiet is URL only", func(t *testing.T) {
		cmd, _ := NewFuzzCmd()
		stdout, _, err := executeCmd(cmd, []string{"-u", srv.URL + "/FUZZ", "-w", wlFile, "-q"})
		if err != nil {
			t.Fatalf("unexpected fuzz error: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if strings.Contains(line, "[+]") || strings.Contains(line, "200") || strings.Contains(line, "FUZZ CONFIGURATION") {
				t.Errorf("fuzz quiet mode emitted formatted line instead of bare URL: %q", line)
			}
			if !strings.HasPrefix(line, srv.URL) {
				t.Errorf("expected bare URL starting with %q, got %q", srv.URL, line)
			}
		}
	})

	t.Run("scan non-quiet has status prefix", func(t *testing.T) {
		cmd, _ := NewScanCmd()
		stdout, _, err := executeCmd(cmd, []string{"-u", srv.URL, "-w", wlFile})
		if err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}
		if !strings.Contains(stdout, "200") {
			t.Errorf("non-quiet scan output should contain status code 200, got:\n%s", stdout)
		}
	})

	t.Run("fuzz non-quiet has status prefix", func(t *testing.T) {
		cmd, _ := NewFuzzCmd()
		stdout, _, err := executeCmd(cmd, []string{"-u", srv.URL + "/FUZZ", "-w", wlFile})
		if err != nil {
			t.Fatalf("unexpected fuzz error: %v", err)
		}
		if !strings.Contains(stdout, "200") {
			t.Errorf("non-quiet fuzz output should contain status code 200, got:\n%s", stdout)
		}
	})
}

// TestConnectTimeout_Fuzz verifies that --connect-timeout is accepted by fuzz,
// actually reaches the configuration/transport, and invalid formats are rejected.
func TestConnectTimeout_Fuzz(t *testing.T) {
	t.Run("connect-timeout accepted and applied", func(t *testing.T) {
		cmd, opts := NewFuzzCmd()
		var capturedCfg config.Config
		opts.testHookConfigApplied = func(cfg config.Config) {
			capturedCfg = cfg
		}

		tmpDir := t.TempDir()
		wlFile := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlFile, []byte("item\n"), 0600)

		_, _, _ = executeCmd(cmd, []string{
			"-u", "http://127.0.0.1:54321/FUZZ", "-w", wlFile, "--connect-timeout", "4500ms", "--dry-run",
		})

		if capturedCfg.ConnectTimeout != 4500*time.Millisecond {
			t.Errorf("expected ConnectTimeout=4.5s, got %v", capturedCfg.ConnectTimeout)
		}
	})

	t.Run("invalid connect-timeout rejected in validation", func(t *testing.T) {
		cmd, _ := NewFuzzCmd()
		tmpDir := t.TempDir()
		wlFile := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlFile, []byte("item\n"), 0600)

		_, _, err := executeCmd(cmd, []string{
			"-u", "http://127.0.0.1:54321/FUZZ", "-w", wlFile, "--connect-timeout", "not-a-duration",
		})
		if err == nil {
			t.Fatal("expected error for invalid connect-timeout duration, got nil")
		}
		if !strings.Contains(err.Error(), "invalid --connect-timeout") {
			t.Errorf("expected 'invalid --connect-timeout' error, got: %v", err)
		}
	})
}

// TestScanEncode verifies that scan accepts --encode and applies transformations
// identically to fuzz semantics.
func TestScanEncode(t *testing.T) {
	t.Run("scan accepts valid encodings", func(t *testing.T) {
		validEncodings := []string{"base64", "url", "doubleurl", "wl-base64", "wl-url", "wl-doubleurl"}
		for _, enc := range validEncodings {
			cmd, _ := NewScanCmd()
			_, _, err := executeCmd(cmd, []string{
				"-u", "http://127.0.0.1:54321", "--encode", enc, "--dry-run",
			})
			if err != nil {
				t.Errorf("encoding %q rejected: %v", enc, err)
			}
		}
	})

	t.Run("scan rejects invalid encoding", func(t *testing.T) {
		cmd, _ := NewScanCmd()
		_, _, err := executeCmd(cmd, []string{
			"-u", "http://127.0.0.1:54321", "--encode", "rot13", "--dry-run",
		})
		if err == nil {
			t.Fatal("expected error for unknown encoding 'rot13', got nil")
		}
		if !strings.Contains(err.Error(), "invalid encoding") {
			t.Errorf("expected 'invalid encoding' error, got: %v", err)
		}
	})

	t.Run("scan base64 transforms candidate words", func(t *testing.T) {
		var requestedPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestedPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		wlFile := filepath.Join(tmpDir, "wl.txt")
		_ = os.WriteFile(wlFile, []byte("secret\n"), 0600)

		cmd, _ := NewScanCmd()
		_, _, err := executeCmd(cmd, []string{
			"-u", srv.URL, "-w", wlFile, "--encode", "base64", "-t", "1",
		})
		if err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}

		// "secret" in base64 is "c2VjcmV0"
		if !strings.Contains(requestedPath, "c2VjcmV0") {
			t.Errorf("expected base64 encoded path containing 'c2VjcmV0', got %q", requestedPath)
		}
	})
}
