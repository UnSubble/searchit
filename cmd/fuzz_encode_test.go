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
	for _, inv := range []string{"hex", "rot13", "none", "utf8", "wl-hex"} {
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
			for _, exp := range []string{"base64", "url", "doubleurl", "wl-base64", "wl-url", "wl-doubleurl"} {
				if !strings.Contains(err.Error(), exp) {
					t.Errorf("error %q does not list supported encoding %q", err.Error(), exp)
				}
			}
		})
	}

	// Valid encodings must pass validation
	for _, valid := range []string{
		"base64", "url", "doubleurl", "BASE64", "Url", "DoubleURL",
		"wl-base64", "wl-url", "wl-doubleurl", "WL-BASE64", "Wl-Url", "WL-DOUBLEURL",
	} {
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

func TestFuzzCLI_WordlistURL_DryRun(t *testing.T) {
	wl := createWordlistFile(t, "admin/user")

	stdout, stderr, err := executeFuzzCmd([]string{
		"-u", "http://example.com/api/FUZZ",
		"-w", wl,
		"-E", "wl-url",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The word admin/user must be URL encoded, and /api/ must remain unchanged
	expected := "http://example.com/api/admin%2Fuser"
	if !strings.Contains(stdout, expected) {
		t.Errorf("expected %q in stdout, got:\n%s", expected, stdout)
	}
	// Static URL must NOT be encoded
	if strings.Contains(stdout, "http%3A%2F%2Fexample.com") {
		t.Errorf("static URL must not be encoded:\n%s", stdout)
	}

	// Configuration output must contain Encoding wl-url
	if !strings.Contains(stderr, "Encoding                     wl-url") && !strings.Contains(stderr, "Encoding                   wl-url") {
		t.Errorf("expected 'Encoding wl-url' in configuration stderr:\n%s", stderr)
	}
}

func TestFuzzCLI_WordlistDoubleURL_DryRun(t *testing.T) {
	wl := createWordlistFile(t, "admin/user")

	stdout, stderr, err := executeFuzzCmd([]string{
		"-u", "http://example.com/api/FUZZ/test",
		"-w", wl,
		"--encode", "wl-doubleurl",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "http://example.com/api/admin%252Fuser/test"
	if !strings.Contains(stdout, expected) {
		t.Errorf("expected %q in stdout, got:\n%s", expected, stdout)
	}
	if strings.Contains(stdout, "http%3A%2F%2Fexample.com") {
		t.Errorf("static URL must not be encoded:\n%s", stdout)
	}

	if !strings.Contains(stderr, "Encoding                     wl-doubleurl") && !strings.Contains(stderr, "Encoding                   wl-doubleurl") {
		t.Errorf("expected 'Encoding wl-doubleurl' in configuration stderr:\n%s", stderr)
	}
}

func TestFuzzCLI_WordlistBase64_DryRun(t *testing.T) {
	wl := createWordlistFile(t, "admin", "admin/user")

	stdout, stderr, err := executeFuzzCmd([]string{
		"-u", "http://example.com/api/FUZZ",
		"-w", wl,
		"-E", "wl-base64",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "http://example.com/api/YWRtaW4=") {
		t.Errorf("expected base64 encoded 'admin' in stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "http://example.com/api/YWRtaW4vdXNlcg==") {
		t.Errorf("expected base64 encoded 'admin/user' in stdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "http%3A%2F%2Fexample.com") {
		t.Errorf("static URL must not be encoded:\n%s", stdout)
	}

	if !strings.Contains(stderr, "Encoding                     wl-base64") && !strings.Contains(stderr, "Encoding                   wl-base64") {
		t.Errorf("expected 'Encoding wl-base64' in configuration stderr:\n%s", stderr)
	}
}

func TestFuzzCLI_WordlistEncoding_Extensions(t *testing.T) {
	wl := createWordlistFile(t, "admin/user")

	stdout, _, err := executeFuzzCmd([]string{
		"-u", "http://example.com/api/FUZZ",
		"-w", wl,
		"-e", ",php",
		"-E", "wl-url",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should produce extensionless variant and .php variant
	if !strings.Contains(stdout, "http://example.com/api/admin%2Fuser\n") {
		t.Errorf("expected extensionless variant in stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "http://example.com/api/admin%2Fuser.php\n") {
		t.Errorf("expected .php variant in stdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "%2Ephp") {
		t.Errorf(".php must not be percent-encoded as %%2Ephp:\n%s", stdout)
	}
}

func TestFuzzCLI_WordlistBase64_Extensions(t *testing.T) {
	wl := createWordlistFile(t, "admin")

	stdout, _, err := executeFuzzCmd([]string{
		"-u", "http://example.com/FUZZ",
		"-w", wl,
		"-e", ",php",
		"-E", "wl-base64",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With wl-base64, the word is base64 encoded first (YWRtaW4=), then extensions are added
	// producing YWRtaW4= and YWRtaW4=.php (in contrast to full-URL base64 which encodes admin.php -> YWRtaW4ucGhw)
	if !strings.Contains(stdout, "http://example.com/YWRtaW4=\n") {
		t.Errorf("expected base64 variant in stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "http://example.com/YWRtaW4=.php\n") {
		t.Errorf("expected base64 with .php extension in stdout:\n%s", stdout)
	}
}

func TestFuzzCLI_WordlistEncoding_Aliases(t *testing.T) {
	wl := createWordlistFile(t, "admin/user")

	stdout, _, err := executeFuzzCmd([]string{
		"-u", "http://example.com/api/FUZZ/FOO",
		"-w", wl,
		"--foo", "=fuzz",
		"-E", "wl-url",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "http://example.com/api/admin%2Fuser/admin%2Fuser"
	if !strings.Contains(stdout, expected) {
		t.Errorf("expected aliased placeholder to receive wl-url encoding:\n got stdout:\n%s\n want match: %s", stdout, expected)
	}
}

func TestFuzzCLI_WordlistEncoding_SecondaryWordlist(t *testing.T) {
	wlFuzz := createWordlistFile(t, "route1")
	wlFoo := createWordlistFile(t, "secret/param")

	stdout, _, err := executeFuzzCmd([]string{
		"-u", "http://example.com/api/FUZZ?data=FOO",
		"-w", wlFuzz,
		"--foo", wlFoo,
		"-E", "wl-url",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "http://example.com/api/route1?data=secret%2Fparam"
	if !strings.Contains(stdout, expected) {
		t.Errorf("expected secondary wordlist to receive wl-url encoding:\n got stdout:\n%s\n want match: %s", stdout, expected)
	}
}

func TestFuzzCLI_WordlistEncoding_HeadersCookiesBody(t *testing.T) {
	wlFuzz := createWordlistFile(t, "user/1")
	wlFoo := createWordlistFile(t, "token/secret")
	wlBar := createWordlistFile(t, "session/cookie")

	stdout, _, err := executeFuzzCmd([]string{
		"-u", "http://example.com/api/FUZZ",
		"-H", "X-Token: FOO",
		"-b", "sid=BAR",
		"-d", "body=FUZZ",
		"-w", wlFuzz,
		"--foo", wlFoo,
		"--bar", wlBar,
		"-E", "wl-url",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "http://example.com/api/user%2F1") {
		t.Errorf("expected encoded URL in stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "X-Token: token%2Fsecret") {
		t.Errorf("expected encoded Header in stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "sid=session%2Fcookie") {
		t.Errorf("expected encoded Cookie in stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "body=user%2F1") {
		t.Errorf("expected encoded Body in stdout:\n%s", stdout)
	}
}

func TestFuzzCLI_WordlistEncoding_RealExecution(t *testing.T) {
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
		"-E", "wl-url",
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
	if !strings.Contains(gotPath, "/api/admin%2Fuser/test") {
		t.Errorf("server received unexpected path: %q, expected to contain '/api/admin%%2Fuser/test'", gotPath)
	}
}
