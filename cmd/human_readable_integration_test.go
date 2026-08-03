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

func TestHumanReadable_ScanAndFuzz(t *testing.T) {
	// Server returning a known size (e.g. 1048576 bytes = 1.0 MB)
	largeBody := strings.Repeat("X", 1048576)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(largeBody))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.txt")
	if err := os.WriteFile(wlPath, []byte("large\n"), 0644); err != nil {
		t.Fatalf("failed to create wordlist: %v", err)
	}

	t.Run("Scan Default Raw Bytes Output", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "scan_raw.txt")
		scanCmd, _ := NewScanCmd()
		scanCmd.SetArgs([]string{"-u", srv.URL, "-w", wlPath, "-o", outFile})

		if err := scanCmd.Execute(); err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}

		data, _ := os.ReadFile(outFile)
		content := string(data)
		if !strings.Contains(content, "1048576 B") {
			t.Errorf("expected raw bytes '1048576 B' in default scan output file, got:\n%s", content)
		}
		if strings.Contains(content, "1.0 MB") {
			t.Errorf("did not expect '1.0 MB' in default raw scan output file, got:\n%s", content)
		}
	})

	t.Run("Scan -R Flag Human-Readable Output", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "scan_hr.txt")
		scanCmd, _ := NewScanCmd()
		scanCmd.SetArgs([]string{"-u", srv.URL, "-w", wlPath, "-R", "-o", outFile})

		if err := scanCmd.Execute(); err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}

		data, _ := os.ReadFile(outFile)
		content := string(data)
		if !strings.Contains(content, "1.0 MB") {
			t.Errorf("expected '1.0 MB' in scan output file with -R, got:\n%s", content)
		}
	})

	t.Run("Scan --human-readable Long Flag Output", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "scan_hr_long.txt")
		scanCmd, _ := NewScanCmd()
		scanCmd.SetArgs([]string{"-u", srv.URL, "-w", wlPath, "--human-readable", "-o", outFile})

		if err := scanCmd.Execute(); err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}

		data, _ := os.ReadFile(outFile)
		content := string(data)
		if !strings.Contains(content, "1.0 MB") {
			t.Errorf("expected '1.0 MB' in scan output file with --human-readable, got:\n%s", content)
		}
	})

	t.Run("Fuzz Default Raw Bytes Output", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "fuzz_raw.txt")
		fuzzCmd, _ := NewFuzzCmd()
		fuzzCmd.SetArgs([]string{"-u", srv.URL + "/FUZZ", "-w", wlPath, "-o", outFile})

		if err := fuzzCmd.Execute(); err != nil {
			t.Fatalf("unexpected fuzz error: %v", err)
		}

		data, _ := os.ReadFile(outFile)
		content := string(data)
		if !strings.Contains(content, "1048576 B") {
			t.Errorf("expected raw bytes '1048576 B' in default fuzz output file, got:\n%s", content)
		}
		if strings.Contains(content, "1.0 MB") {
			t.Errorf("did not expect '1.0 MB' in default raw fuzz output file, got:\n%s", content)
		}
	})

	t.Run("Fuzz -R Flag Human-Readable Output", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "fuzz_hr.txt")
		fuzzCmd, _ := NewFuzzCmd()
		fuzzCmd.SetArgs([]string{"-u", srv.URL + "/FUZZ", "-w", wlPath, "-R", "-o", outFile})

		if err := fuzzCmd.Execute(); err != nil {
			t.Fatalf("unexpected fuzz error: %v", err)
		}

		data, _ := os.ReadFile(outFile)
		content := string(data)
		if !strings.Contains(content, "1.0 MB") {
			t.Errorf("expected '1.0 MB' in fuzz output file with -R, got:\n%s", content)
		}
	})

	t.Run("Fuzz --human-readable Long Flag Output", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "fuzz_hr_long.txt")
		fuzzCmd, _ := NewFuzzCmd()
		fuzzCmd.SetArgs([]string{"-u", srv.URL + "/FUZZ", "-w", wlPath, "--human-readable", "-o", outFile})

		if err := fuzzCmd.Execute(); err != nil {
			t.Fatalf("unexpected fuzz error: %v", err)
		}

		data, _ := os.ReadFile(outFile)
		content := string(data)
		if !strings.Contains(content, "1.0 MB") {
			t.Errorf("expected '1.0 MB' in fuzz output file with --human-readable, got:\n%s", content)
		}
	})

	t.Run("Structured JSON Output Retains Raw Numeric Length With -R", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "scan_json.json")
		scanCmd, _ := NewScanCmd()
		scanCmd.SetArgs([]string{"-u", srv.URL, "-w", wlPath, "-R", "--format", "json", "-o", outFile})

		if err := scanCmd.Execute(); err != nil {
			t.Fatalf("unexpected scan error: %v", err)
		}

		data, _ := os.ReadFile(outFile)
		var results []map[string]any
		if err := json.Unmarshal(data, &results); err != nil {
			t.Fatalf("failed to unmarshal JSON results: %v (raw: %s)", err, string(data))
		}
		if len(results) == 0 {
			t.Fatalf("expected results in JSON output")
		}
		lengthVal, ok := results[0]["length"].(float64)
		if !ok || int64(lengthVal) != 1048576 {
			t.Errorf("expected numeric length 1048576 in JSON output, got: %v", results[0]["length"])
		}
	})
}
