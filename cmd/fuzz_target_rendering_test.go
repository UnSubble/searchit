package cmd_test

import (
	"bytes"
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

func createTinyWordlist(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	if err := os.WriteFile(wlPath, []byte("testword\n"), 0600); err != nil {
		t.Fatalf("failed to create tiny wordlist: %v", err)
	}
	return wlPath
}

func runFuzzCmdAndCaptureStdoutAndStderr(t *testing.T, args []string) (string, string) {
	t.Helper()

	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		_ = rOut.Close()
		_ = wOut.Close()
		t.Fatalf("os.Pipe: %v", err)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
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

	cxx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = fuzzCmd.ExecuteContext(cxx)

	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	wg.Wait()
	_ = rOut.Close()
	_ = rErr.Close()

	return stdoutBuf.String(), stderrBuf.String()
}

func runFuzzCmdAndCaptureStderr(t *testing.T, args []string) string {
	_, stderr := runFuzzCmdAndCaptureStdoutAndStderr(t, args)
	return stderr
}

func TestFuzz_ContextAwareTargetRendering_URLOnly(t *testing.T) {
	wl := createTinyWordlist(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	out := runFuzzCmdAndCaptureStderr(t, []string{
		"-u", srv.URL + "/FUZZ",
		"-w", wl,
		"-t", "1",
		"--no-progress",
	})

	if !strings.Contains(out, "Fuzz Target\n\n  URL\n    "+srv.URL+"/FUZZ") {
		t.Fatalf("expected Fuzz Target URL section in output, got:\n%s", out)
	}
	if strings.Contains(out, "  Header") || strings.Contains(out, "  Cookie") || strings.Contains(out, "  Body") {
		t.Errorf("expected no Header, Cookie, or Body sections in output, got:\n%s", out)
	}
}

func TestFuzz_ContextAwareTargetRendering_HeaderOnly(t *testing.T) {
	wl := createTinyWordlist(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	out := runFuzzCmdAndCaptureStderr(t, []string{
		"-u", srv.URL,
		"-w", wl,
		"-H", "Host: FUZZ.futurevera.thm",
		"-t", "1",
		"--no-progress",
	})

	expectedTarget := "Fuzz Target\n\n  URL\n    " + srv.URL + "\n\n  Header\n    Host: FUZZ.futurevera.thm"
	if !strings.Contains(out, expectedTarget) {
		t.Fatalf("expected Fuzz Target with URL and Header in output, got:\n%s", out)
	}
	if strings.Contains(out, "  Cookie") || strings.Contains(out, "  Body") {
		t.Errorf("expected no Cookie or Body sections, got:\n%s", out)
	}
}

func TestFuzz_ContextAwareTargetRendering_URLAndHeader(t *testing.T) {
	wl := createTinyWordlist(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	out := runFuzzCmdAndCaptureStderr(t, []string{
		"-u", srv.URL + "/FUZZ/api",
		"-w", wl,
		"-H", "Authorization: Bearer BAR",
		"--bar", "=fuzz",
		"-t", "1",
		"--no-progress",
	})

	expectedTarget := "Fuzz Target\n\n  URL\n    " + srv.URL + "/FUZZ/api\n\n  Header\n    Authorization: Bearer BAR"
	if !strings.Contains(out, expectedTarget) {
		t.Fatalf("expected Fuzz Target with URL and Header in output, got:\n%s", out)
	}
	if strings.Contains(out, "  Cookie") || strings.Contains(out, "  Body") {
		t.Errorf("expected no Cookie or Body sections, got:\n%s", out)
	}
}

func TestFuzz_ContextAwareTargetRendering_MultipleHeaders(t *testing.T) {
	wl := createTinyWordlist(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	out := runFuzzCmdAndCaptureStderr(t, []string{
		"-u", srv.URL,
		"-w", wl,
		"-H", "Host: FUZZ.example.com",
		"-H", "X-Normal: ignore-me",
		"-H", "Authorization: Bearer BAR",
		"--bar", "=fuzz",
		"-t", "1",
		"--no-progress",
	})

	expectedHeaders := "  Header\n    Host: FUZZ.example.com\n    Authorization: Bearer BAR"
	if !strings.Contains(out, expectedHeaders) {
		t.Fatalf("expected multiple placeholder headers in output, got:\n%s", out)
	}
	if strings.Contains(out, "X-Normal") {
		t.Errorf("header without placeholder should not be in Fuzz Target section:\n%s", out)
	}
}

func TestFuzz_ContextAwareTargetRendering_CookieOnly(t *testing.T) {
	wl := createTinyWordlist(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	out := runFuzzCmdAndCaptureStderr(t, []string{
		"-u", srv.URL,
		"-w", wl,
		"-b", "session=FUZZ",
		"-t", "1",
		"--no-progress",
	})

	expectedTarget := "Fuzz Target\n\n  URL\n    " + srv.URL + "\n\n  Cookie\n    session=FUZZ"
	if !strings.Contains(out, expectedTarget) {
		t.Fatalf("expected Fuzz Target with URL and Cookie in output, got:\n%s", out)
	}
	if strings.Contains(out, "  Header") || strings.Contains(out, "  Body") {
		t.Errorf("expected no Header or Body sections, got:\n%s", out)
	}
}

func TestFuzz_ContextAwareTargetRendering_BodyOnly(t *testing.T) {
	wl := createTinyWordlist(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	out := runFuzzCmdAndCaptureStderr(t, []string{
		"-u", srv.URL,
		"-w", wl,
		"-d", `{"user":"FUZZ"}`,
		"-t", "1",
		"--no-progress",
	})

	expectedTarget := "Fuzz Target\n\n  URL\n    " + srv.URL + "\n\n  Body\n    {\"user\":\"FUZZ\"}"
	if !strings.Contains(out, expectedTarget) {
		t.Fatalf("expected Fuzz Target with URL and Body in output, got:\n%s", out)
	}
	if strings.Contains(out, "  Header") || strings.Contains(out, "  Cookie") {
		t.Errorf("expected no Header or Cookie sections, got:\n%s", out)
	}
}

func TestFuzz_ContextAwareTargetRendering_Query(t *testing.T) {
	wl := createTinyWordlist(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	out := runFuzzCmdAndCaptureStderr(t, []string{
		"-u", srv.URL + "?id=FUZZ",
		"-w", wl,
		"-t", "1",
		"--no-progress",
	})

	expectedTarget := "Fuzz Target\n\n  URL\n    " + srv.URL + "?id=FUZZ"
	if !strings.Contains(out, expectedTarget) {
		t.Fatalf("expected Fuzz Target with Query in URL in output, got:\n%s", out)
	}
	if strings.Contains(out, "  Header") || strings.Contains(out, "  Cookie") || strings.Contains(out, "  Body") {
		t.Errorf("expected no Header, Cookie, or Body sections, got:\n%s", out)
	}
}

func TestFuzz_ContextAwareTargetRendering_Mixed(t *testing.T) {
	wl := createTinyWordlist(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	out := runFuzzCmdAndCaptureStderr(t, []string{
		"-u", srv.URL + "/FUZZ?search=FOO",
		"-w", wl,
		"-H", "Host: BUZZ.example.com",
		"-b", "session=BAR",
		"-d", `{"item":"BAZ"}`,
		"--foo", "=fuzz",
		"--bar", "=fuzz",
		"--baz", "=fuzz",
		"--buzz", "=fuzz",
		"-t", "1",
		"--no-progress",
	})

	expectedSections := []string{
		"Fuzz Target",
		"  URL\n    " + srv.URL + "/FUZZ?search=FOO",
		"  Header\n    Host: BUZZ.example.com",
		"  Cookie\n    session=BAR",
		"  Body\n    {\"item\":\"BAZ\"}",
	}

	for _, s := range expectedSections {
		if !strings.Contains(out, s) {
			t.Errorf("expected section %q in output, got:\n%s", s, out)
		}
	}
}

func TestFuzz_ContextAwareTargetRendering_QuietMode(t *testing.T) {
	wl := createTinyWordlist(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	for _, quietFlag := range []string{"-q", "--quiet"} {
		t.Run(quietFlag, func(t *testing.T) {
			stdout, stderr := runFuzzCmdAndCaptureStdoutAndStderr(t, []string{
				"-u", srv.URL + "/FUZZ",
				"-w", wl,
				"-H", "Host: FUZZ.futurevera.thm",
				quietFlag,
				"-t", "1",
			})

			if strings.Contains(stderr, "Fuzz Target") || strings.Contains(stderr, "FUZZ CONFIGURATION") {
				t.Errorf("expected no Fuzz Target or FUZZ CONFIGURATION in stderr with %s, got:\n%s", quietFlag, stderr)
			}
			if strings.Contains(stdout, "Fuzz Target") || strings.Contains(stdout, "FUZZ CONFIGURATION") {
				t.Errorf("expected no Fuzz Target or FUZZ CONFIGURATION in stdout with %s, got:\n%s", quietFlag, stdout)
			}
		})
	}
}

func TestFuzz_ContextAwareTargetRendering_StructuredFormats(t *testing.T) {
	wl := createTinyWordlist(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	formats := []string{"json", "ndjson", "csv", "markdown"}
	for _, fmtName := range formats {
		t.Run(fmtName, func(t *testing.T) {
			stdout, _ := runFuzzCmdAndCaptureStdoutAndStderr(t, []string{
				"-u", srv.URL + "/FUZZ",
				"-w", wl,
				"-H", "Host: FUZZ.futurevera.thm",
				"--format", fmtName,
				"-t", "1",
				"--no-progress",
			})

			if strings.Contains(stdout, "Fuzz Target") {
				t.Errorf("expected no Fuzz Target section in structured format %s output, got:\n%s", fmtName, stdout)
			}
		})
	}
}
