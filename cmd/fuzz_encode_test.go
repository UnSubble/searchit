package cmd_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/cmd"
)

func createWordlistFile(t *testing.T, lines ...string) string {
	t.Helper()
	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(wlPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to create wordlist: %v", err)
	}
	return wlPath
}

func executeFuzzCmd(args []string) (string, string, error) {
	rOut, wOut, err := os.Pipe()
	if err != nil {
		return "", "", err
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		_ = rOut.Close()
		_ = wOut.Close()
		return "", "", err
	}

	var stdoutBuf, stderrBuf strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stdoutBuf, rOut)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stderrBuf, rErr)
	}()

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdout = wOut
	os.Stderr = wErr

	fuzzCmd, _ := cmd.NewFuzzCmd()
	fuzzCmd.SetOut(wOut)
	fuzzCmd.SetErr(wErr)
	fuzzCmd.SetArgs(args)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmdErr := fuzzCmd.ExecuteContext(ctx)

	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	wg.Wait()
	_ = rOut.Close()
	_ = rErr.Close()

	return stdoutBuf.String(), stderrBuf.String(), cmdErr
}

func TestFuzzCLI_EncodeValidation(t *testing.T) {
	wl := createWordlistFile(t, "admin")

	// Invalid encodings must fail
	for _, inv := range []string{"hex", "rot13", "none", "utf8"} {
		t.Run("invalid_"+inv, func(t *testing.T) {
			_, _, err := executeFuzzCmd([]string{
				"-u", "http://example.com/FUZZ",
				"-w", wl,
				"--encode", inv,
				"--dry-run",
			})
			if err == nil {
				t.Fatalf("expected error for --encode %q, got nil", inv)
			}
			if !strings.Contains(err.Error(), "supported encodings are base64, url, doubleurl") {
				t.Errorf("error %q does not list supported encodings", err.Error())
			}
		})
	}

	// Valid encodings must pass validation
	for _, valid := range []string{"base64", "url", "doubleurl", "BASE64", "Url", "DoubleURL"} {
		t.Run("valid_"+valid, func(t *testing.T) {
			_, _, err := executeFuzzCmd([]string{
				"-u", "http://example.com/FUZZ",
				"-w", wl,
				"-E", valid,
				"--dry-run",
			})
			if err != nil {
				t.Fatalf("expected valid encoding %q to succeed, got error: %v", valid, err)
			}
		})
	}
}

func TestFuzzCLI_Default_NoEncoding(t *testing.T) {
	wl := createWordlistFile(t, "admin/user")

	stdout, stderr, err := executeFuzzCmd([]string{
		"-u", "http://example.com/api/FUZZ/test",
		"-w", wl,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Candidate must remain unencoded
	if !strings.Contains(stdout, "http://example.com/api/admin/user/test") {
		t.Errorf("expected unaltered candidate URL in stdout:\n%s", stdout)
	}

	// Configuration output must NOT contain Encoding item when not specified
	if strings.Contains(stderr, "\nEncoding ") || strings.Contains(stderr, "Encoding     ") {
		t.Errorf("expected no Encoding line in configuration output, got:\n%s", stderr)
	}
}

func TestFuzzCLI_DryRun_URL(t *testing.T) {
	wl := createWordlistFile(t, "admin/user")

	// Test short flag -E
	stdout, stderr, err := executeFuzzCmd([]string{
		"-u", "http://example.com/api/FUZZ/test",
		"-w", wl,
		"-E", "url",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// URL-encoded candidate
	if !strings.Contains(stdout, "http://example.com/api/admin%2Fuser/test") {
		t.Errorf("expected URL-encoded candidate in stdout:\n%s", stdout)
	}

	// Configuration output must contain Encoding url
	if !strings.Contains(stderr, "Encoding                     url") && !strings.Contains(stderr, "Encoding                   url") {
		t.Errorf("expected 'Encoding url' in configuration stderr:\n%s", stderr)
	}
}

func TestFuzzCLI_DryRun_DoubleURL(t *testing.T) {
	wl := createWordlistFile(t, "admin/user")

	stdout, stderr, err := executeFuzzCmd([]string{
		"-u", "http://example.com/api/FUZZ/test",
		"-w", wl,
		"--encode", "doubleurl",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "http://example.com/api/admin%252Fuser/test") {
		t.Errorf("expected double URL-encoded candidate in stdout:\n%s", stdout)
	}

	if !strings.Contains(stderr, "Encoding                     doubleurl") && !strings.Contains(stderr, "Encoding                   doubleurl") {
		t.Errorf("expected 'Encoding doubleurl' in configuration stderr:\n%s", stderr)
	}
}

func TestFuzzCLI_DryRun_Base64(t *testing.T) {
	wl := createWordlistFile(t, "admin/user")

	stdout, stderr, err := executeFuzzCmd([]string{
		"-u", "http://example.com/api/FUZZ/test",
		"-w", wl,
		"-E", "base64",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "http://example.com/api/YWRtaW4vdXNlcg==/test") {
		t.Errorf("expected base64-encoded candidate in stdout:\n%s", stdout)
	}

	if !strings.Contains(stderr, "Encoding                     base64") && !strings.Contains(stderr, "Encoding                   base64") {
		t.Errorf("expected 'Encoding base64' in configuration stderr:\n%s", stderr)
	}
}

func TestFuzzCLI_Extensions_WithEncoding(t *testing.T) {
	wl := createWordlistFile(t, "admin/user")

	stdout, _, err := executeFuzzCmd([]string{
		"-u", "http://example.com/FUZZ",
		"-w", wl,
		"-e", ",php",
		"-E", "url",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both empty extension and .php extension must be generated, with dot preserved
	if !strings.Contains(stdout, "http://example.com/admin%2Fuser\n") {
		t.Errorf("expected extensionless variant in stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "http://example.com/admin%2Fuser.php\n") {
		t.Errorf("expected .php variant in stdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "test%2Ephp") || strings.Contains(stdout, "%2Ephp") {
		t.Errorf("dot should NOT be encoded in extension variant, got:\n%s", stdout)
	}
}

func TestFuzzCLI_MultiplePlaceholders_WithEncoding(t *testing.T) {
	wlFuzz := createWordlistFile(t, "user/1")
	wlFoo := createWordlistFile(t, "role/admin")

	stdout, _, err := executeFuzzCmd([]string{
		"-u", "http://example.com/api/FUZZ?role=FOO",
		"-w", wlFuzz,
		"--foo", wlFoo,
		"-E", "url",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "http://example.com/api/user%2F1?role=role%2Fadmin"
	if !strings.Contains(stdout, expected) {
		t.Errorf("expected both placeholders encoded:\n got stdout:\n%s\n want match: %s", stdout, expected)
	}
}

func TestFuzzCLI_RealExecution_EncodedRequest(t *testing.T) {
	wl := createWordlistFile(t, "admin/user")

	var receivedPath string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedPath = r.URL.RequestURI()
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	_, _, err := executeFuzzCmd([]string{
		"-u", srv.URL + "/api/FUZZ/test",
		"-w", wl,
		"-E", "url",
		"-t", "1",
		"--no-progress",
	})
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}

	mu.Lock()
	gotPath := receivedPath
	mu.Unlock()

	// Server should receive the URL-encoded path
	if !strings.Contains(gotPath, "/api/admin%2Fuser/test") && !strings.Contains(gotPath, "admin%2Fuser") {
		t.Errorf("server received unexpected path: %q", gotPath)
	}
}

func TestFuzzCLI_RealExecution_HeaderPlaceholderEncoded(t *testing.T) {
	wl := createWordlistFile(t, "secret:token")

	var receivedHeader string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedHeader = r.Header.Get("X-Auth")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, _, err := executeFuzzCmd([]string{
		"-u", srv.URL + "/index",
		"-H", "X-Auth: Basic FUZZ",
		"-w", wl,
		"-E", "base64",
		"-t", "1",
		"--no-progress",
	})
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}

	mu.Lock()
	gotHeader := receivedHeader
	mu.Unlock()

	// "secret:token" base64-encoded is "c2VjcmV0OnRva2Vu"
	expected := "Basic c2VjcmV0OnRva2Vu"
	if gotHeader != expected {
		t.Errorf("expected header %q, got %q", expected, gotHeader)
	}
}

func TestFuzzCLI_RealExecution_BodyPlaceholderEncoded(t *testing.T) {
	wl := createWordlistFile(t, "user/name")

	var receivedBody string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBody = string(b)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, _, err := executeFuzzCmd([]string{
		"-u", srv.URL + "/submit",
		"-d", "username=FUZZ",
		"-w", wl,
		"-E", "doubleurl",
		"-t", "1",
		"--no-progress",
	})
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}

	mu.Lock()
	gotBody := receivedBody
	mu.Unlock()

	// "user/name" double-url-encoded is "user%252Fname"
	expected := "username=user%252Fname"
	if gotBody != expected {
		t.Errorf("expected body %q, got %q", expected, gotBody)
	}
}

func TestFuzzCLI_Alias_WithEncoding(t *testing.T) {
	wl := createWordlistFile(t, "admin/user")

	stdout, _, err := executeFuzzCmd([]string{
		"-u", "http://example.com/api/FUZZ/FOO",
		"-w", wl,
		"--foo", "=fuzz",
		"-E", "url",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "http://example.com/api/admin%2Fuser/admin%2Fuser"
	if !strings.Contains(stdout, expected) {
		t.Errorf("expected aliased placeholder to receive encoding:\n got stdout:\n%s\n want match: %s", stdout, expected)
	}
}
