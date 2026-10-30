package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/color"
)

func TestBoldStyling_Scan(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		case "/redir":
			http.Redirect(w, r, "/target", http.StatusFound)
		case "/err":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("error"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	_ = os.WriteFile(wlPath, []byte("ok\nredir\nerr\n"), 0600)

	t.Run("bold enabled by default when color output enabled (TTY auto)", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return true })
		defer color.ResetTTYDetector()

		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "--no-progress"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		if !color.HasBold(output) {
			t.Errorf("expected bold ANSI codes when color is enabled on TTY, got:\n%s", output)
		}
		// Bold green indicator [+] and status 200
		if !strings.Contains(output, "\033[1;32m[+]\033[0m") {
			t.Errorf("expected bold green [+] badge, got:\n%s", output)
		}
		if !strings.Contains(output, "\033[1;32m200\033[0m") {
			t.Errorf("expected bold green 200, got:\n%s", output)
		}
		// Byte received should be bold white (\033[1;37m2 B\033[0m)
		if !strings.Contains(output, "\033[1;37m2 B\033[0m -") {
			t.Errorf("expected bold white '2 B' in output, got:\n%s", output)
		}
		// URL must NOT be bold
		if strings.Contains(output, "\033[1m"+ts.URL) || strings.Contains(output, "\033[1;32m"+ts.URL) {
			t.Errorf("URL should remain normal weight, got:\n%s", output)
		}
	})

	t.Run("bold disabled with --color never", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return true })
		defer color.ResetTTYDetector()

		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "--color", "never", "--no-progress"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		if color.HasANSI(output) || color.HasBold(output) {
			t.Errorf("expected NO ANSI or bold with --color never, got:\n%s", output)
		}
		if !strings.Contains(output, "[+] 200") {
			t.Errorf("expected plain '[+] 200', got:\n%s", output)
		}
	})

	t.Run("bold disabled in automatic non-TTY mode", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return false })
		defer color.ResetTTYDetector()

		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "--no-progress"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		if color.HasANSI(output) || color.HasBold(output) {
			t.Errorf("expected NO ANSI or bold in auto mode without TTY, got:\n%s", output)
		}
	})

	t.Run("explicit --color always enables bold even on non-TTY", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return false })
		defer color.ResetTTYDetector()

		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "--color", "always", "--no-progress"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		if !color.HasBold(output) {
			t.Errorf("expected bold styling with --color always, got:\n%s", output)
		}
		if !strings.Contains(output, "\033[1;32m[+]\033[0m") {
			t.Errorf("expected bold green [+], got:\n%s", output)
		}
	})

	t.Run("quiet mode behavior: URL remains normal weight", func(t *testing.T) {
		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "-q", "--color", "always", "--no-progress"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		// Must have color (green finding)
		if !color.HasANSI(output) {
			t.Errorf("expected green finding URL with --color always in quiet mode")
		}
		// But must NOT have bold
		if color.HasBold(output) {
			t.Errorf("quiet mode finding URL must NOT have bold weight, got:\n%s", output)
		}
		if !strings.Contains(output, "\033[32m"+ts.URL+"/ok\033[0m") {
			t.Errorf("expected normal-weight green URL, got:\n%s", output)
		}
	})

	t.Run("combined bold and semantic colors for redirects", func(t *testing.T) {
		cmd, _ := NewScanCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "--color", "always", "--no-progress"})

		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		output := out.String()
		// 302 redirect should have bold yellow: \033[1;33m[302]\033[0m
		if !strings.Contains(output, "\033[1;33m[302]\033[0m") {
			t.Errorf("expected bold yellow [302] status code, got:\n%s", output)
		}
		// Byte received should be bold white
		if !strings.Contains(output, "\033[1;37m30 B\033[0m") {
			t.Errorf("expected bold white '30 B' in redirect row, got:\n%s", output)
		}
		// Arrow -> should be bold white
		if !strings.Contains(output, "\033[1;37m->\033[0m") {
			t.Errorf("expected bold white arrow -> in redirect row, got:\n%s", output)
		}
	})
}

func TestBoldStyling_ScanAndFuzzParity(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("found"))
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	_ = os.WriteFile(wlPath, []byte("test\n"), 0600)

	// Scan output with color always
	cmdScan, _ := NewScanCmd()
	var outScan bytes.Buffer
	cmdScan.SetOut(&outScan)
	cmdScan.SetErr(&outScan)
	cmdScan.SetArgs([]string{"-u", ts.URL, "-w", wlPath, "--color", "always", "--no-progress"})
	if err := cmdScan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Fuzz output with color always
	cmdFuzz, _ := NewFuzzCmd()
	var outFuzz bytes.Buffer
	cmdFuzz.SetOut(&outFuzz)
	cmdFuzz.SetErr(&outFuzz)
	cmdFuzz.SetArgs([]string{"-u", ts.URL + "/FUZZ", "-w", wlPath, "--color", "always", "--no-progress"})
	if err := cmdFuzz.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("fuzz failed: %v", err)
	}

	scanStr := outScan.String()
	fuzzStr := outFuzz.String()

	// Both must contain identical bold green [+] finding badge
	if !strings.Contains(scanStr, "\033[1;32m[+]\033[0m") {
		t.Errorf("scan missing bold green [+]:\n%s", scanStr)
	}
	if !strings.Contains(fuzzStr, "\033[1;32m[+]\033[0m") {
		t.Errorf("fuzz missing bold green [+]:\n%s", fuzzStr)
	}

	// Both must contain identical bold green 200 status code
	if !strings.Contains(scanStr, "\033[1;32m200\033[0m") {
		t.Errorf("scan missing bold green 200:\n%s", scanStr)
	}
	if !strings.Contains(fuzzStr, "\033[1;32m200\033[0m") {
		t.Errorf("fuzz missing bold green 200:\n%s", fuzzStr)
	}

	// Both must contain bold white byte received label
	if !strings.Contains(scanStr, "\033[1;37m5 B\033[0m") {
		t.Errorf("scan missing bold white byte size:\n%s", scanStr)
	}
	if !strings.Contains(fuzzStr, "\033[1;37m5 B\033[0m") {
		t.Errorf("fuzz missing bold white byte size:\n%s", fuzzStr)
	}

	// Neither must bold the URL
	if strings.Contains(scanStr, "\033[1m"+ts.URL) || strings.Contains(scanStr, "\033[1;32m"+ts.URL) {
		t.Errorf("scan should not bold URL:\n%s", scanStr)
	}
	if strings.Contains(fuzzStr, "\033[1m"+ts.URL) || strings.Contains(fuzzStr, "\033[1;32m"+ts.URL) {
		t.Errorf("fuzz should not bold URL:\n%s", fuzzStr)
	}
}

func TestBoldStyling_Doctor(t *testing.T) {
	defer resetRootFlags()

	t.Run("doctor color always has bold headings and status", func(t *testing.T) {
		resetRootFlags()
		rootCmd.SetArgs([]string{"doctor", "--color", "always"})
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)

		_ = rootCmd.ExecuteContext(context.Background())
		output := out.String()

		// Title heading must be bold
		if !strings.Contains(output, "\033[1msearchit doctor\033[0m") {
			t.Errorf("expected bold 'searchit doctor' heading, got:\n%s", output)
		}
		// Status label must be bold
		if !strings.Contains(output, "\033[1mSTATUS\033[0m:") {
			t.Errorf("expected bold 'STATUS' heading, got:\n%s", output)
		}
		// Check results must have bold status (e.g. bold green PASS)
		if !strings.Contains(output, "\033[1;32mPASS\033[0m") {
			t.Errorf("expected bold green PASS in doctor output, got:\n%s", output)
		}
	})

	t.Run("doctor color never has NO bold and NO ANSI", func(t *testing.T) {
		color.SetTTYDetector(func() bool { return true })
		defer color.ResetTTYDetector()

		resetRootFlags()
		rootCmd.SetArgs([]string{"doctor", "--color", "never"})
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)

		_ = rootCmd.ExecuteContext(context.Background())
		output := out.String()

		if color.HasANSI(output) || color.HasBold(output) {
			t.Errorf("expected NO ANSI or bold in doctor with --color never, got:\n%s", output)
		}
		if !strings.Contains(output, "searchit doctor") {
			t.Errorf("expected plain 'searchit doctor' header, got:\n%s", output)
		}
		if !strings.Contains(output, "STATUS:") {
			t.Errorf("expected plain 'STATUS:' label, got:\n%s", output)
		}
	})
}

func TestBoldStyling_StructuredOutputClean(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	_ = os.WriteFile(wlPath, []byte("test\n"), 0600)

	formats := []string{"json", "csv", "ndjson"}
	for _, fmtName := range formats {
		t.Run("format "+fmtName+" with --color always has no ANSI and no bold", func(t *testing.T) {
			outPath := filepath.Join(tmpDir, fmt.Sprintf("out.%s", fmtName))

			cmd, _ := NewScanCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{
				"-u", ts.URL,
				"-w", wlPath,
				"-o", outPath,
				"--format", fmtName,
				"--color", "always",
				"--no-progress",
			})

			if err := cmd.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("scan with format %s failed: %v", fmtName, err)
			}

			fileData, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("failed to read output file: %v", err)
			}

			fileStr := string(fileData)
			if color.HasANSI(fileStr) || color.HasBold(fileStr) {
				t.Errorf("format %s output file contains ANSI/bold escape sequences:\n%s", fmtName, fileStr)
			}

			// Validate JSON parses cleanly
			if fmtName == "json" {
				var parsed []map[string]any
				if err := json.Unmarshal(fileData, &parsed); err != nil {
					t.Errorf("JSON parse failed: %v\nContent:\n%s", err, fileStr)
				}
			}
		})
	}
}
