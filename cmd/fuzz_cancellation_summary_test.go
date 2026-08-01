package cmd_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/unsubble/searchit/cmd"
)

func TestFuzz_CancellationRendersSummary(t *testing.T) {
	// Start a slow mock server to allow canceling during fuzzing
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer srv.Close()

	cxx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	fuzzCmd, _ := cmd.NewFuzzCmd()

	// Capture stderr where summary is printed
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	fuzzCmd.SetArgs([]string{
		"-u", srv.URL + "/FUZZ",
		"-w", "internal/wordlist/embedded.txt",
		"-t", "2",
		"--no-progress", // ensure test environment doesn't rely on TTY
	})

	err := fuzzCmd.ExecuteContext(cxx)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()

	if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify that FUZZ SUMMARY is printed even when canceled
	if !strings.Contains(out, "FUZZ SUMMARY") {
		t.Errorf("expected output to contain 'FUZZ SUMMARY' on cancellation, got:\n%s", out)
	}
}

func TestScan_CancellationRendersSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer srv.Close()

	cxx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	scanCmd, _ := cmd.NewScanCmd()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	scanCmd.SetArgs([]string{
		"-u", srv.URL,
		"-w", "internal/wordlist/embedded.txt",
		"-t", "2",
		"--no-progress",
	})

	err := scanCmd.ExecuteContext(cxx)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()

	if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "SCAN SUMMARY") {
		t.Errorf("expected output to contain 'SCAN SUMMARY' on cancellation, got:\n%s", out)
	}
}
