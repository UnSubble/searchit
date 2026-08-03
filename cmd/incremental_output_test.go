package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestIncrementalOutputFileWriting_LiveWriting verifies that accepted findings
// are written incrementally to the output file while the scan is actively running,
// rather than being buffered until scan completion.
func TestIncrementalOutputFileWriting_LiveWriting(t *testing.T) {
	gate := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/FUZZ" || r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(r.URL.Path, "hit") {
			w.WriteHeader(http.StatusOK)
			return
		}
		// Request for slow blocks until gate is released
		<-gate
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "live_output.json")
	wordlistFile := filepath.Join(tmpDir, "words.txt")
	_ = os.WriteFile(wordlistFile, []byte("hit\nslow\n"), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd, _ := NewScanCmd()
	cmd.SetArgs([]string{
		"-u", srv.URL + "/FUZZ",
		"-w", wordlistFile,
		"-o", outFile,
		"--format", "json",
		"-t", "1",
	})

	errChan := make(chan error, 1)
	go func() {
		errChan <- cmd.ExecuteContext(ctx)
	}()

	// Wait for the first finding to hit
	deadline := time.Now().Add(15 * time.Second)
	var foundLive bool
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		data, err := os.ReadFile(outFile)
		if err == nil && len(data) > 0 && strings.Contains(string(data), "hit") {
			foundLive = true
			break
		}
	}

	close(gate)
	if !foundLive {
		t.Fatal("expected finding 'hit' to appear in output file while scan is still running, but file was empty or missing")
	}
}

// TestIncrementalOutputFileWriting_PartialScan verifies that if a scan is aborted or cancelled mid-way,
// the output file contains all findings emitted up to that point and remains a valid JSON file.
func TestIncrementalOutputFileWriting_PartialScan(t *testing.T) {
	gate := make(chan struct{})

	var hitReceived int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "hit1") {
			atomic.StoreInt32(&hitReceived, 1)
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/FUZZ" || r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if atomic.LoadInt32(&hitReceived) == 1 {
			<-gate
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "partial_output.json")
	wordlistFile := filepath.Join(tmpDir, "words.txt")
	_ = os.WriteFile(wordlistFile, []byte("hit1\nslow1\nslow2\n"), 0644)

	ctx, cancel := context.WithCancel(context.Background())

	cmd, _ := NewScanCmd()
	cmd.SetArgs([]string{
		"-u", srv.URL + "/FUZZ",
		"-w", wordlistFile,
		"-o", outFile,
		"--format", "json",
		"-t", "5",
	})

	errChan := make(chan error, 1)
	go func() {
		errChan <- cmd.ExecuteContext(ctx)
	}()

	// Wait until hit1 is written to the file
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		data, err := os.ReadFile(outFile)
		if err == nil && strings.Contains(string(data), "hit1") {
			break
		}
	}

	// Cancel context to simulate Ctrl+C / abort
	cancel()
	close(gate)
	<-errChan

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	if !strings.Contains(string(data), "hit1") {
		t.Errorf("expected partial scan output file to contain 'hit1', got: %s", string(data))
	}

	// Verify that the partial output file is valid JSON
	var parsed []map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("expected partial output file to be valid JSON, unmarshal failed: %v\ncontent:\n%s", err, string(data))
	}
}

// TestIncrementalOutputFileWriting_FinalOutputIdentical verifies that incremental
// file emission produces identical final output to completed scans.
func TestIncrementalOutputFileWriting_FinalOutputIdentical(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "page1") || strings.Contains(r.URL.Path, "page2") {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "final_output.json")
	wordlistFile := filepath.Join(tmpDir, "words.txt")
	_ = os.WriteFile(wordlistFile, []byte("page1\npage2\nmissing\n"), 0644)

	cmd, _ := NewScanCmd()
	cmd.SetArgs([]string{
		"-u", srv.URL + "/FUZZ",
		"-w", wordlistFile,
		"-o", outFile,
		"--format", "json",
		"-t", "2",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute failed: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("failed to parse final JSON output: %v\ncontent:\n%s", err, string(data))
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results in JSON output file, got %d", len(results))
	}
}
