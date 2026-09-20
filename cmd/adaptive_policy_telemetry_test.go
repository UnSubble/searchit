package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecursiveScanAdaptivePolicyTelemetry_NotConflated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "", "/", "/sub":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("dir"))
		case "/sub/child":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("child"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	origDebug := flagDebug
	flagDebug = true
	defer func() { flagDebug = origDebug }()

	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	_ = os.WriteFile(wlPath, []byte("sub\nchild\nmissing\n"), 0644)

	scanCmd, _ := NewScanCmd()
	var outBuf bytes.Buffer
	scanCmd.SetOut(&outBuf)
	scanCmd.SetErr(&outBuf)

	scanCmd.SetArgs([]string{
		"-u", srv.URL,
		"-w", wlPath,
		"-r",
		"--max-depth", "2",
		"--strategy", "dfs",
		"--adaptive",
	})

	err := scanCmd.ExecuteContext(context.Background())
	if err != nil {
		t.Fatalf("scan execution failed: %v\noutput:\n%s", err, outBuf.String())
	}

	outStr := outBuf.String()

	if !strings.Contains(outStr, "ADAPTIVE SUMMARY") {
		t.Fatalf("expected ADAPTIVE SUMMARY in output, got:\n%s", outStr)
	}

	if strings.Contains(outStr, "DFS Policies") {
		// When ADAPTIVE SUMMARY is printed, DFS Policies must be 0, not the number of discovered directories.
		for _, line := range strings.Split(outStr, "\n") {
			lineTrimmed := strings.TrimSpace(line)
			if strings.HasPrefix(lineTrimmed, "DFS Policies") {
				if !strings.Contains(lineTrimmed, "0") {
					t.Errorf("expected DFS Policies to be 0, got line: %q", lineTrimmed)
				}
			}
			if strings.HasPrefix(lineTrimmed, "BFS Policies") {
				if !strings.Contains(lineTrimmed, "0") {
					t.Errorf("expected BFS Policies to be 0, got line: %q", lineTrimmed)
				}
			}
			if strings.HasPrefix(lineTrimmed, "Eager Policies") {
				if !strings.Contains(lineTrimmed, "0") {
					t.Errorf("expected Eager Policies to be 0, got line: %q", lineTrimmed)
				}
			}
		}
	}
}
