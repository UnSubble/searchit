package cmd_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/unsubble/searchit/cmd"
)

func TestFUZZPrimaryPlaceholder_Regression(t *testing.T) {
	// Custom wordlist with 3 words
	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	err := os.WriteFile(wlPath, []byte("val1\nval2\nval3\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write wordlist: %v", err)
	}

	rawReqPath := filepath.Join(tmpDir, "request.txt")
	reqContent := "POST /api HTTP/1.1\r\nHost: example.com\r\nX-Test: FUZZ\r\n\r\n"
	if err := os.WriteFile(rawReqPath, []byte(reqContent), 0644); err != nil {
		t.Fatalf("failed to write raw request: %v", err)
	}

	tests := []struct {
		name                string
		args                func(srvURL string) []string
		verifyRequest       func(r *http.Request) string
		expectedCount       int
		expectedSubstitutes []string
	}{
		{
			name: "FUZZ in URL",
			args: func(srvURL string) []string {
				return []string{"fuzz", "-u", srvURL + "/FUZZ", "-w", wlPath, "-t", "1", "-q"}
			},
			verifyRequest: func(r *http.Request) string {
				return r.URL.Path[1:] // Trim leading slash
			},
			expectedCount:       3,
			expectedSubstitutes: []string{"val1", "val2", "val3"},
		},
		{
			name: "FUZZ in Header",
			args: func(srvURL string) []string {
				return []string{"fuzz", "-u", srvURL + "/test", "-H", "X-Custom: FUZZ", "-w", wlPath, "-t", "1", "-q"}
			},
			verifyRequest: func(r *http.Request) string {
				return r.Header.Get("X-Custom")
			},
			expectedCount:       3,
			expectedSubstitutes: []string{"val1", "val2", "val3"},
		},
		{
			name: "FUZZ in Cookie",
			args: func(srvURL string) []string {
				return []string{"fuzz", "-u", srvURL + "/test", "-b", "sess=FUZZ", "-w", wlPath, "-t", "1", "-q"}
			},
			verifyRequest: func(r *http.Request) string {
				c, err := r.Cookie("sess")
				if err != nil {
					return ""
				}
				return c.Value
			},
			expectedCount:       3,
			expectedSubstitutes: []string{"val1", "val2", "val3"},
		},
		{
			name: "FUZZ in Body",
			args: func(srvURL string) []string {
				return []string{"fuzz", "-u", srvURL + "/test", "-X", "POST", "-d", "username=FUZZ", "-w", wlPath, "-t", "1", "-q"}
			},
			verifyRequest: func(r *http.Request) string {
				b, _ := io.ReadAll(r.Body)
				return string(b)
			},
			expectedCount:       3,
			expectedSubstitutes: []string{"username=val1", "username=val2", "username=val3"},
		},
		{
			name: "FUZZ in Raw Request Template",
			args: func(srvURL string) []string {
				rawReqFile := filepath.Join(tmpDir, "request_sub.txt")
				_ = os.WriteFile(rawReqFile, []byte("POST "+srvURL+"/api HTTP/1.1\r\nX-Test: FUZZ\r\n\r\n"), 0644)
				return []string{"fuzz", "--request", rawReqFile, "-w", wlPath, "-t", "1", "-q"}
			},
			verifyRequest: func(r *http.Request) string {
				return r.Header.Get("X-Test")
			},
			expectedCount:       3,
			expectedSubstitutes: []string{"val1", "val2", "val3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var received []string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sub := tt.verifyRequest(r)
				mu.Lock()
				received = append(received, sub)
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			cmdObj, _ := cmd.NewFuzzCmd()
			cmdObj.SetArgs(tt.args(srv.URL))

			var outBuf, errBuf bytes.Buffer
			cmdObj.SetOut(&outBuf)
			cmdObj.SetErr(&errBuf)

			err := cmdObj.Execute()
			if err != nil {
				t.Fatalf("command failed: %v, stderr: %s", err, errBuf.String())
			}

			mu.Lock()
			defer mu.Unlock()

			if len(received) != tt.expectedCount {
				t.Fatalf("expected %d requests, got %d: %v", tt.expectedCount, len(received), received)
			}

			sortedReceived := append([]string(nil), received...)
			sort.Strings(sortedReceived)

			sortedExpected := append([]string(nil), tt.expectedSubstitutes...)
			sort.Strings(sortedExpected)

			for i, exp := range sortedExpected {
				if sortedReceived[i] != exp {
					t.Errorf("request %d: expected %q, got %q", i, exp, sortedReceived[i])
				}
			}
		})
	}
}

func TestFUZZEmbeddedWordlist_CandidateConsistency(t *testing.T) {
	// Test that without -w flag, candidate count defaults to 4751 for embedded wordlist
	// regardless of whether FUZZ is in URL, Header, Cookie, Body, or Raw Request Template.
	rawReqDir := t.TempDir()

	testCases := []struct {
		name string
		args func(srvURL string) []string
	}{
		{
			name: "Embedded FUZZ in URL",
			args: func(srvURL string) []string {
				return []string{"fuzz", "-u", srvURL + "/FUZZ"}
			},
		},
		{
			name: "Embedded FUZZ in Header",
			args: func(srvURL string) []string {
				return []string{"fuzz", "-u", srvURL + "/test", "-H", "Content-Type: FUZZ"}
			},
		},
		{
			name: "Embedded FUZZ in Cookie",
			args: func(srvURL string) []string {
				return []string{"fuzz", "-u", srvURL + "/test", "-b", "session=FUZZ"}
			},
		},
		{
			name: "Embedded FUZZ in Body",
			args: func(srvURL string) []string {
				return []string{"fuzz", "-u", srvURL + "/test", "-X", "POST", "-d", "username=FUZZ"}
			},
		},
		{
			name: "Embedded FUZZ in Raw Request Template",
			args: func(srvURL string) []string {
				rawReqPath := filepath.Join(rawReqDir, "req.txt")
				_ = os.WriteFile(rawReqPath, []byte("GET "+srvURL+"/ HTTP/1.1\r\nX-Fuzz: FUZZ\r\n\r\n"), 0644)
				return []string{"fuzz", "--request", rawReqPath}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var requestCount int
			var mu sync.Mutex

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requestCount++
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			cmdObj, _ := cmd.NewFuzzCmd()
			cmdObj.SetArgs(tc.args(srv.URL))

			var outBuf, errBuf bytes.Buffer
			cmdObj.SetOut(&outBuf)
			cmdObj.SetErr(&errBuf)

			err := cmdObj.Execute()
			if err != nil {
				t.Fatalf("command failed: %v, stderr: %s", err, errBuf.String())
			}

			mu.Lock()
			count := requestCount
			mu.Unlock()

			if count != 4751 {
				t.Errorf("expected 4751 requests, got %d", count)
			}
		})
	}
}
