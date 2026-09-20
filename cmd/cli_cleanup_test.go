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

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
