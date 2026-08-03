package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedirectFinalSizeReporting(t *testing.T) {
	// Create final destination HTTP server returning 8432 bytes
	finalBody := strings.Repeat("A", 8432)
	finalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(finalBody))
	}))
	defer finalSrv.Close()

	// Create redirect chain server (301 -> 302 -> finalSrv)
	redir302Srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalSrv.URL+"/", http.StatusFound)
	}))
	defer redir302Srv.Close()

	redir301Srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redir302Srv.URL+"/", http.StatusMovedPermanently)
	}))
	defer redir301Srv.Close()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	os.WriteFile(wlPath, []byte("admin\n"), 0644)

	t.Run("Scenario 1 & 2: Single 302 redirect with --follow-redirects reports 302 and final body size (8432 B)", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "out1.txt")
		scanCmd, _ := NewScanCmd()
		scanCmd.SetArgs([]string{"-u", redir302Srv.URL, "-w", wlPath, "--follow-redirects", "-o", outFile})

		err := scanCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, _ := os.ReadFile(outFile)
		outStr := string(content)
		if !strings.Contains(outStr, "[302] - 8432 B") {
			t.Errorf("expected '[302] - 8432 B' in output file, got: %s", outStr)
		}
	})

	t.Run("Scenario 1 & 2 (Human Readable): Single 302 redirect with --follow-redirects -R reports 302 and final body size (8.2 KB)", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "out1_hr.txt")
		scanCmd, _ := NewScanCmd()
		scanCmd.SetArgs([]string{"-u", redir302Srv.URL, "-w", wlPath, "--follow-redirects", "-R", "-o", outFile})

		err := scanCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, _ := os.ReadFile(outFile)
		outStr := string(content)
		if !strings.Contains(outStr, "[302] - 8.2 KB") {
			t.Errorf("expected '[302] - 8.2 KB' in output file with -R, got: %s", outStr)
		}
	})

	t.Run("Scenario 3: Redirects disabled (without --follow-redirects) reports original 302 size (46 B)", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "out2.txt")
		scanCmd, _ := NewScanCmd()
		scanCmd.SetArgs([]string{"-u", redir302Srv.URL, "-w", wlPath, "-o", outFile})

		err := scanCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, _ := os.ReadFile(outFile)
		outStr := string(content)
		if !strings.Contains(outStr, "[302] - 46 B") {
			t.Errorf("expected '[302] - 46 B' in output file when redirects are disabled, got: %s", outStr)
		}
	})

	t.Run("Scenario 4: Redirect chain (301 -> 302 -> 200) reports initial 301 status and final body size (8432 B)", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "out3.txt")
		scanCmd, _ := NewScanCmd()
		scanCmd.SetArgs([]string{"-u", redir301Srv.URL, "-w", wlPath, "--follow-redirects", "-o", outFile})

		err := scanCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, _ := os.ReadFile(outFile)
		outStr := string(content)
		if !strings.Contains(outStr, "[301] - 8432 B") {
			t.Errorf("expected '[301] - 8432 B' for redirect chain in output file, got: %s", outStr)
		}
	})

	t.Run("Scenario 4 (Human Readable): Redirect chain (301 -> 302 -> 200) with --human-readable reports (8.2 KB)", func(t *testing.T) {
		outFile := filepath.Join(tmpDir, "out3_hr.txt")
		scanCmd, _ := NewScanCmd()
		scanCmd.SetArgs([]string{"-u", redir301Srv.URL, "-w", wlPath, "--follow-redirects", "--human-readable", "-o", outFile})

		err := scanCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, _ := os.ReadFile(outFile)
		outStr := string(content)
		if !strings.Contains(outStr, "[301] - 8.2 KB") {
			t.Errorf("expected '[301] - 8.2 KB' for redirect chain in output file with --human-readable, got: %s", outStr)
		}
	})
}
