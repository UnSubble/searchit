package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/unsubble/searchit/internal/color"
)

func resetRootFlags() {
	rootCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		_ = f.Value.Set(f.DefValue)
	})
	flagColor = "auto"
	verbose = false
	flagDebug = false
}

func TestColor_Scan(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok response"))
		case "/redir":
			http.Redirect(w, r, "/ok", http.StatusFound)
		case "/error":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("error response"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	if err := os.WriteFile(wlPath, []byte("ok\nredir\n"), 0o600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	t.Run("auto mode when non-TTY produces no ANSI", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return false })
		defer color.ResetTTYDetector()

		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		if color.HasANSI(output) {
			t.Errorf("expected no ANSI codes in auto mode on non-TTY, got:\n%s", output)
		}
		if !strings.Contains(output, "[+] 200") {
			t.Errorf("expected finding in output, got:\n%s", output)
		}
	})

	t.Run("auto mode when TTY produces ANSI codes", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return true })
		defer color.ResetTTYDetector()

		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		if !color.HasANSI(output) {
			t.Errorf("expected ANSI codes in auto mode on TTY, got:\n%s", output)
		}
		if !strings.Contains(output, "\033[1;32m[+]\033[0m") {
			t.Errorf("expected bold green [+] badge in output, got:\n%s", output)
		}
		if !strings.Contains(output, "\033[1;32m200\033[0m") {
			t.Errorf("expected bold green 200 status code in output, got:\n%s", output)
		}
	})

	t.Run("explicit always forces colors even on non-TTY", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return false })
		defer color.ResetTTYDetector()

		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "--color", "always"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		if !color.HasANSI(output) {
			t.Errorf("expected ANSI codes with --color always, got:\n%s", output)
		}
		if !strings.Contains(output, "\033[1;32m[+]\033[0m") {
			t.Errorf("expected bold green [+] badge with --color always, got:\n%s", output)
		}
	})

	t.Run("explicit never disables colors even on TTY", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return true })
		defer color.ResetTTYDetector()

		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "--color", "never"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		if color.HasANSI(output) {
			t.Errorf("expected NO ANSI codes with --color never, got:\n%s", output)
		}
		if !strings.Contains(output, "[+] 200") {
			t.Errorf("expected plain finding with --color never, got:\n%s", output)
		}
	})

	t.Run("invalid color value returns clean error", func(t *testing.T) {
		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "--color", "magenta"})

		err := cmd.ExecuteContext(context.Background())
		if err == nil {
			t.Fatalf("expected error with invalid --color, got nil")
		}
		if !strings.Contains(err.Error(), "invalid --color option") {
			t.Errorf("expected 'invalid --color option' error message, got: %v", err)
		}
	})

	t.Run("quiet mode auto-disables colors even on TTY", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return true })
		defer color.ResetTTYDetector()

		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "-q"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		if color.HasANSI(output) {
			t.Errorf("expected no ANSI codes in quiet mode (auto), got:\n%s", output)
		}
		if !strings.Contains(output, ts.URL+"/ok") {
			t.Errorf("expected URL in quiet mode output, got:\n%s", output)
		}
	})

	t.Run("quiet mode with color always preserves colors", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return false })
		defer color.ResetTTYDetector()

		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "-q", "--color", "always"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		if !color.HasANSI(output) {
			t.Errorf("expected ANSI codes with -q --color always, got:\n%s", output)
		}
		if !strings.Contains(output, "\033[32m"+ts.URL+"/ok\033[0m") {
			t.Errorf("expected green URL with -q --color always, got:\n%s", output)
		}
	})

	t.Run("file output via -o remains clean with color always", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return false })
		defer color.ResetTTYDetector()

		outFile := filepath.Join(tmpDir, "clean_results.txt")
		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "-o", outFile, "--color", "always"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		// Terminal output should have ANSI
		if !color.HasANSI(out.String()) {
			t.Errorf("expected terminal stdout to have ANSI with --color always")
		}

		// File output must NEVER have ANSI
		fileData, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		fileStr := string(fileData)
		if color.HasANSI(fileStr) {
			t.Errorf("expected file output to be completely clean of ANSI codes, got:\n%s", fileStr)
		}
		if !strings.Contains(fileStr, "[+] 200") {
			t.Errorf("expected plain findings in output file, got:\n%s", fileStr)
		}
	})

	t.Run("structured format JSON never contains ANSI even with color always", func(t *testing.T) {
		jsonFile := filepath.Join(tmpDir, "results.json")
		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "-o", jsonFile, "--format", "json", "--color", "always"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		// Stdout format was json so even on stdout it should have no ANSI
		if color.HasANSI(output) {
			t.Errorf("expected JSON stdout to NOT contain ANSI codes, got:\n%s", output)
		}

		fileData, err := os.ReadFile(jsonFile)
		if err != nil {
			t.Fatalf("failed to read JSON output file: %v", err)
		}
		if color.HasANSI(string(fileData)) {
			t.Errorf("expected JSON file to NOT contain ANSI codes, got:\n%s", string(fileData))
		}

		var parsed []map[string]interface{}
		if err := json.Unmarshal(fileData, &parsed); err != nil {
			t.Errorf("expected valid JSON file, got unmarshal error: %v\nContent was: %s", err, string(fileData))
		}
	})

	t.Run("structured format CSV never contains ANSI even with color always", func(t *testing.T) {
		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "--format", "csv", "--color", "always"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		if color.HasANSI(output) {
			t.Errorf("expected CSV output to NOT contain ANSI codes, got:\n%s", output)
		}
		if !strings.Contains(output, "200") {
			t.Errorf("expected CSV to contain status 200, got:\n%s", output)
		}
	})

	t.Run("root command flag color always before scan", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return false })
		defer color.ResetTTYDetector()

		resetRootFlags()
		rootCmd.SetArgs([]string{"--color", "always", "scan", "-u", ts.URL, "-w", wlPath})
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)

		if err := rootCmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan via root failed: %v", err)
		}

		output := out.String()
		if !color.HasANSI(output) {
			t.Errorf("expected ANSI when --color always passed before scan, got:\n%s", output)
		}
	})

	t.Run("root command invalid color option", func(t *testing.T) {
		resetRootFlags()
		rootCmd.SetArgs([]string{"--color", "invalid", "scan", "-u", ts.URL, "-w", wlPath})
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)

		err := rootCmd.ExecuteContext(context.Background())
		if err == nil {
			t.Fatalf("expected error when invalid color passed to root, got nil")
		}
		if !strings.Contains(err.Error(), "invalid --color option") {
			t.Errorf("expected 'invalid --color option' error, got: %v", err)
		}
	})
}

func TestColor_Fuzz(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("admin"))
		case "/login":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("login"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "fuzz_words.txt")
	if err := os.WriteFile(wlPath, []byte("admin\nlogin\n"), 0o600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	t.Run("fuzz auto mode non-TTY has no ANSI", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return false })
		defer color.ResetTTYDetector()

		cmd, _ := NewFuzzCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL + "/FUZZ", "-w", wlPath})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("fuzz failed: %v", err)
		}

		output := out.String()
		if color.HasANSI(output) {
			t.Errorf("expected no ANSI codes in fuzz auto non-TTY, got:\n%s", output)
		}
		if !strings.Contains(output, "[+] 200") {
			t.Errorf("expected finding in output, got:\n%s", output)
		}
	})

	t.Run("fuzz color always forces ANSI", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return false })
		defer color.ResetTTYDetector()

		cmd, _ := NewFuzzCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL + "/FUZZ", "-w", wlPath, "--color", "always"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("fuzz failed: %v", err)
		}

		output := out.String()
		if !color.HasANSI(output) {
			t.Errorf("expected ANSI codes in fuzz with --color always, got:\n%s", output)
		}
		if !strings.Contains(output, "\033[1;32m[+]\033[0m") {
			t.Errorf("expected bold green [+] in fuzz output, got:\n%s", output)
		}
	})

	t.Run("fuzz quiet with color always", func(t *testing.T) {
		cmd, _ := NewFuzzCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL + "/FUZZ", "-w", wlPath, "-q", "--color", "always"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("fuzz failed: %v", err)
		}

		output := out.String()
		if !color.HasANSI(output) {
			t.Errorf("expected ANSI codes in fuzz quiet with --color always, got:\n%s", output)
		}
		if !strings.Contains(output, "\033[32m"+ts.URL+"/admin\033[0m") {
			t.Errorf("expected green URL in quiet mode with --color always, got:\n%s", output)
		}
	})

	t.Run("fuzz invalid color option", func(t *testing.T) {
		cmd, _ := NewFuzzCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL + "/FUZZ", "-w", wlPath, "--color", "invalid"})

		err := cmd.ExecuteContext(context.Background())
		if err == nil {
			t.Fatalf("expected error for invalid --color in fuzz, got nil")
		}
		if !strings.Contains(err.Error(), "invalid --color option") {
			t.Errorf("expected 'invalid --color option' error, got: %v", err)
		}
	})

	t.Run("fuzz file output via -o remains clean with color always", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "fuzz_clean.txt")
		cmd, _ := NewFuzzCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL + "/FUZZ", "-w", wlPath, "-o", outFile, "--color", "always"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("fuzz failed: %v", err)
		}

		fileData, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		if color.HasANSI(string(fileData)) {
			t.Errorf("expected fuzz file output to be ANSI-free, got:\n%s", string(fileData))
		}
	})
}

func TestColor_Doctor(t *testing.T) {
	defer resetRootFlags()

	t.Run("doctor auto non-TTY has no ANSI", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return false })
		defer color.ResetTTYDetector()

		resetRootFlags()
		rootCmd.SetArgs([]string{"doctor"})
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)

		_ = rootCmd.ExecuteContext(context.Background())
		output := out.String()
		if color.HasANSI(output) {
			t.Errorf("expected doctor auto non-TTY to have no ANSI, got:\n%s", output)
		}
		if !strings.Contains(output, "PASS") {
			t.Errorf("expected PASS in doctor output, got:\n%s", output)
		}
		if !strings.Contains(output, "STATUS:") {
			t.Errorf("expected STATUS: in doctor output, got:\n%s", output)
		}
	})

	t.Run("doctor color always has ANSI", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return false })
		defer color.ResetTTYDetector()

		resetRootFlags()
		rootCmd.SetArgs([]string{"doctor", "--color", "always"})
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)

		_ = rootCmd.ExecuteContext(context.Background())
		output := out.String()
		if !color.HasANSI(output) {
			t.Errorf("expected doctor --color always to have ANSI, got:\n%s", output)
		}
		if !strings.Contains(output, "\033[1;32mPASS\033[0m") {
			t.Errorf("expected bold green PASS in doctor output, got:\n%s", output)
		}
		if !strings.Contains(output, "\033[1msearchit doctor\033[0m") {
			t.Errorf("expected bold 'searchit doctor' heading in doctor output, got:\n%s", output)
		}
		if !strings.Contains(output, "\033[1mSTATUS\033[0m:") {
			t.Errorf("expected bold 'STATUS' heading in doctor output, got:\n%s", output)
		}
	})

	t.Run("doctor color never has no ANSI", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return true })
		defer color.ResetTTYDetector()

		resetRootFlags()
		rootCmd.SetArgs([]string{"doctor", "--color", "never"})
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)

		_ = rootCmd.ExecuteContext(context.Background())
		output := out.String()
		if color.HasANSI(output) {
			t.Errorf("expected doctor --color never to have NO ANSI, got:\n%s", output)
		}
	})

	t.Run("doctor invalid color option", func(t *testing.T) {
		resetRootFlags()
		rootCmd.SetArgs([]string{"doctor", "--color", "unknown"})
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)

		err := rootCmd.ExecuteContext(context.Background())
		if err == nil {
			t.Fatalf("expected error with doctor --color unknown, got nil")
		}
		if !strings.Contains(err.Error(), "invalid --color option") {
			t.Errorf("expected invalid --color option error, got: %v", err)
		}
	})
}

func TestColor_Version(t *testing.T) {
	defer resetRootFlags()

	t.Run("version color always outputs ANSI", func(t *testing.T) {
		resetRootFlags()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `[{"tag_name": "v0.7.0", "draft": false}]`)
		}))
		defer server.Close()
		t.Setenv("SEARCHIT_API_BASE", server.URL)

		// Capture os.Stdout since version prints via fmt.Printf to os.Stdout
		oldStdout := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("failed to create pipe: %v", err)
		}
		os.Stdout = w

		rootCmd.SetArgs([]string{"version", "--color", "always"})
		execErr := rootCmd.ExecuteContext(context.Background())

		w.Close()
		os.Stdout = oldStdout

		var outBuf bytes.Buffer
		io.Copy(&outBuf, r)
		r.Close()

		if execErr != nil {
			t.Fatalf("version failed: %v", execErr)
		}

		out := outBuf.String()
		if !color.HasANSI(out) {
			t.Errorf("expected ANSI in version with --color always, got:\n%s", out)
		}
	})

	t.Run("version color never outputs no ANSI", func(t *testing.T) {
		resetRootFlags()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `[{"tag_name": "v0.7.0", "draft": false}]`)
		}))
		defer server.Close()
		t.Setenv("SEARCHIT_API_BASE", server.URL)

		oldStdout := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("failed to create pipe: %v", err)
		}
		os.Stdout = w

		rootCmd.SetArgs([]string{"version", "--color", "never"})
		execErr := rootCmd.ExecuteContext(context.Background())

		w.Close()
		os.Stdout = oldStdout

		var outBuf bytes.Buffer
		io.Copy(&outBuf, r)
		r.Close()

		if execErr != nil {
			t.Fatalf("version failed: %v", execErr)
		}

		out := outBuf.String()
		if color.HasANSI(out) {
			t.Errorf("expected no ANSI in version with --color never, got:\n%s", out)
		}
	})
}
