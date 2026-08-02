package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestScanRegression_RecursiveScanEmbeddedWordlistCandidates(t *testing.T) {
	var reqCount int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	var buf bytes.Buffer
	scanCmd, _ := NewScanCmd()
	scanCmd.SetOut(&buf)
	scanCmd.SetArgs([]string{"-u", ts.URL, "-t", "16", "-r", "-d", "2", "--quiet"})

	err := scanCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected scan execution error: %v", err)
	}

	count := atomic.LoadInt32(&reqCount)
	// The embedded wordlist contains thousands of words.
	// When root URL returns 404, the scanner must still generate wordlist candidates (> 100).
	if count <= 1 {
		t.Fatalf("REGRESSION DETECTED: recursive scan generated only %d candidates, want > 100", count)
	}
	t.Logf("Regression test passed: generated %d candidates on 404 root target", count)
}
