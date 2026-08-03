package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuietModeFileOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "":
			w.WriteHeader(http.StatusOK)
		case "/admin":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("admin page"))
		case "/redirect":
			http.Redirect(w, r, "/admin", http.StatusFound)
		case "/robots.txt":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /secret\n"))
		case "/secret":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("secret page"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	if err := os.WriteFile(wlPath, []byte("admin\nredirect\n"), 0600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	t.Run("-o alone (scan)", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "out1.txt")
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "-o", outFile,
		})
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		// Terminal should have formatted output
		if !strings.Contains(stdout, "[+] 200") {
			t.Errorf("expected stdout to contain [+] 200 findings when -o is used without -q, got:\n%s", stdout)
		}
		// File should contain standard text format (e.g. [+] 200...)
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		fileStr := string(content)
		if !strings.Contains(fileStr, "[+] 200") {
			t.Errorf("expected standard text format in file without -q, got:\n%s", fileStr)
		}
	})

	t.Run("-q alone (scan)", func(t *testing.T) {
		stdout, stderr, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "-q",
		})
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		// Stderr should be empty (no banner/telemetry)
		if stderr != "" {
			t.Errorf("expected empty stderr in quiet mode, got: %q", stderr)
		}
		// Stdout should be minimal links-only
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		for _, line := range lines {
			if line != "" && !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
				t.Errorf("expected link-only output in stdout, got line: %q", line)
			}
		}
	})

	t.Run("-q + -o default text format (scan)", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "out_quiet.txt")
		stdout, stderr, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "-q", "-o", outFile,
		})
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		// Stderr should be empty (no banner/telemetry)
		if stderr != "" {
			t.Errorf("expected empty stderr when -q is passed, got: %q", stderr)
		}
		// Stdout should contain minimal links-only (terminal sink is NOT disabled by -q -o)
		linesStdout := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(linesStdout) < 1 {
			t.Errorf("expected links-only findings in stdout for -q -o, got empty stdout")
		}
		for _, line := range linesStdout {
			if line != "" && !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
				t.Errorf("stdout line is not a clean URL: %q", line)
			}
		}

		// File should contain ONLY clean links
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		fileStr := string(content)
		if strings.Contains(fileStr, "[+]") || strings.Contains(fileStr, "[302]") {
			t.Errorf("file should not contain formatted text markers in quiet mode, got:\n%s", fileStr)
		}
		linesFile := strings.Split(strings.TrimSpace(fileStr), "\n")
		if len(linesFile) < 1 {
			t.Errorf("expected at least 1 finding in file, got %d", len(linesFile))
		}
		for _, line := range linesFile {
			if line != "" && !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
				t.Errorf("file line is not a clean URL: %q", line)
			}
		}
	})

	t.Run("-q + -o with explicit format --format json", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "out_quiet.json")
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "-q", "-o", outFile, "--format", "json",
		})
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		// Stdout should contain links-only terminal output
		linesStdout := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(linesStdout) < 1 {
			t.Errorf("expected links-only output in stdout with -q, got empty stdout")
		}

		// File should contain valid JSON array
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read JSON output file: %v", err)
		}
		var results []map[string]any
		if err := json.Unmarshal(content, &results); err != nil {
			t.Fatalf("file should contain valid JSON despite -q, got error: %v\nContent:\n%s", err, string(content))
		}
		if len(results) < 1 {
			t.Errorf("expected JSON results in file, got %d items", len(results))
		}
	})

	t.Run("redirects with -q + -o (scan)", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "out_redirect.txt")
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "-q", "-o", outFile, "-x", "",
		})
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		// Check stdout for links-only
		if !strings.Contains(stdout, srv.URL+"/redirect") {
			t.Errorf("quiet stdout should contain redirect URL, got:\n%s", stdout)
		}
		// Check file for links-only
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		fileStr := string(content)
		if strings.Contains(fileStr, "[302]") || strings.Contains(fileStr, "->") {
			t.Errorf("quiet file output should not contain redirect arrow or status prefix, got:\n%s", fileStr)
		}
		if !strings.Contains(fileStr, srv.URL+"/redirect") {
			t.Errorf("quiet file output should contain the redirect target URL, got:\n%s", fileStr)
		}
	})

	t.Run("adaptive fuzzing with -q + -o", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "out_adaptive.txt")
		stdout, stderr, err := runIntegrationCommandStreams([]string{
			"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "-q", "-o", outFile, "--adaptive",
		})
		if err != nil {
			t.Fatalf("fuzz failed: %v", err)
		}
		if stderr != "" {
			t.Errorf("expected quiet stderr, got stderr=%q", stderr)
		}
		if !strings.Contains(stdout, srv.URL+"/admin") {
			t.Errorf("quiet stdout missing finding, got:\n%s", stdout)
		}
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		fileStr := string(content)
		if strings.Contains(fileStr, "[+]") {
			t.Errorf("quiet adaptive fuzz file output should not contain [+] markers, got:\n%s", fileStr)
		}
		if !strings.Contains(fileStr, srv.URL+"/admin") {
			t.Errorf("quiet adaptive fuzz file missing finding, got:\n%s", fileStr)
		}
	})

	t.Run("recursive scan with -q + -o", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "out_recursive.txt")
		stdout, stderr, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "-q", "-o", outFile, "-r", "--max-depth", "2",
		})
		if err != nil {
			t.Fatalf("recursive scan failed: %v", err)
		}
		if stderr != "" {
			t.Errorf("expected quiet stderr, got stderr=%q", stderr)
		}
		if !strings.Contains(stdout, srv.URL+"/admin") {
			t.Errorf("quiet stdout missing finding, got:\n%s", stdout)
		}
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		fileStr := string(content)
		if strings.Contains(fileStr, "[+]") {
			t.Errorf("quiet recursive scan file output should not contain [+] markers, got:\n%s", fileStr)
		}
		if !strings.Contains(fileStr, srv.URL+"/admin") {
			t.Errorf("quiet recursive scan file missing finding, got:\n%s", fileStr)
		}
	})
}

func TestSinkSelectionMatrix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "":
			w.WriteHeader(http.StatusOK)
		case "/admin":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	if err := os.WriteFile(wlPath, []byte("admin\n"), 0600); err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	t.Run("terminal only (none)", func(t *testing.T) {
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath,
		})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}
		if !strings.Contains(stdout, "[+] 200") {
			t.Errorf("expected terminal stdout to contain [+] 200, got:\n%s", stdout)
		}
	})

	t.Run("terminal + file (-o without -q)", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "term_file.txt")
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "-o", outFile,
		})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}
		if !strings.Contains(stdout, "[+] 200") {
			t.Errorf("terminal output MUST NOT disappear when -o is present without -q, got:\n%s", stdout)
		}
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		if !strings.Contains(string(content), "[+] 200") {
			t.Errorf("file output should contain [+] 200, got:\n%s", string(content))
		}
	})

	t.Run("links-only terminal (-q without -o)", func(t *testing.T) {
		stdout, stderr, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "-q",
		})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}
		if stderr != "" {
			t.Errorf("expected empty stderr for -q, got: %q", stderr)
		}
		if !strings.Contains(stdout, srv.URL+"/admin") || strings.Contains(stdout, "[+]") {
			t.Errorf("expected links-only terminal output for -q, got stdout:\n%s", stdout)
		}
	})

	t.Run("links-only terminal + links-only file (-q -o)", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "file_q_o.txt")
		stdout, stderr, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "-q", "-o", outFile,
		})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}
		if stderr != "" {
			t.Errorf("expected empty stderr for -q -o, got stderr=%q", stderr)
		}
		if !strings.Contains(stdout, srv.URL+"/admin") || strings.Contains(stdout, "[+]") {
			t.Errorf("terminal stdout MUST contain links-only output for -q -o, got:\n%s", stdout)
		}
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		if !strings.Contains(string(content), srv.URL+"/admin") || strings.Contains(string(content), "[+]") {
			t.Errorf("file SHOULD contain links-only output for -q -o, got:\n%s", string(content))
		}
	})

	t.Run("adaptive fuzz + -o (without -q)", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "adaptive_out.txt")
		stdout, _, err := runIntegrationCommandStreams([]string{
			"fuzz", "-u", srv.URL + "/FUZZ", "-w", wlPath, "-o", outFile, "--adaptive",
		})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}
		if !strings.Contains(stdout, "[+] 200") {
			t.Errorf("terminal stdout MUST NOT disappear when -o is present in adaptive fuzz, got:\n%s", stdout)
		}
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		if !strings.Contains(string(content), "[+] 200") {
			t.Errorf("file should contain finding, got:\n%s", string(content))
		}
	})

	t.Run("recursive scan + -o (without -q)", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "recursive_out.txt")
		stdout, _, err := runIntegrationCommandStreams([]string{
			"scan", "-u", srv.URL, "-w", wlPath, "-o", outFile, "-r", "--max-depth", "2",
		})
		if err != nil {
			t.Fatalf("command failed: %v", err)
		}
		if !strings.Contains(stdout, "[+] 200") {
			t.Errorf("terminal stdout MUST NOT disappear when -o is present in recursive scan, got:\n%s", stdout)
		}
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		if !strings.Contains(string(content), "[+] 200") {
			t.Errorf("file should contain finding, got:\n%s", string(content))
		}
	})
}
