package cmd_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createWordlistWith(t *testing.T, words ...string) string {
	t.Helper()
	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "words.txt")
	content := strings.Join(words, "\n") + "\n"
	if err := os.WriteFile(wlPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to create wordlist: %v", err)
	}
	return wlPath
}

func runFuzzCmdAndCaptureStdout(t *testing.T, args []string) string {
	stdout, _ := runFuzzCmdAndCaptureStdoutAndStderr(t, args)
	return stdout
}

func TestFuzz_FindingRendering_URLOnly(t *testing.T) {
	wl := createWordlistWith(t, "admin")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"-u", srv.URL + "/FUZZ",
		"-w", wl,
		"-t", "1",
		"--no-progress",
	})

	if !strings.Contains(stdout, "[+] 200 - 2 B") {
		t.Errorf("expected status and size header in finding, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "  URL\n    "+srv.URL+"/admin") {
		t.Errorf("expected rendered URL block, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "Header") || strings.Contains(stdout, "Cookie") || strings.Contains(stdout, "Body") {
		t.Errorf("expected unchanged components not to be rendered, got:\n%s", stdout)
	}
}

func TestFuzz_FindingRendering_Header(t *testing.T) {
	wl := createWordlistWith(t, "custom-header-val")
	var receivedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Custom-Token")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"-u", srv.URL,
		"-H", "X-Custom-Token: FUZZ",
		"-w", wl,
		"-t", "1",
		"--no-progress",
	})

	if receivedHeader != "custom-header-val" {
		t.Errorf("expected wire header to be %q, got %q", "custom-header-val", receivedHeader)
	}

	if !strings.Contains(stdout, "  URL\n    "+srv.URL) {
		t.Errorf("expected URL block in finding, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "  Header\n    X-Custom-Token: custom-header-val") {
		t.Errorf("expected Header block in finding, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "Cookie") || strings.Contains(stdout, "Body") {
		t.Errorf("expected unchanged components not to be rendered, got:\n%s", stdout)
	}
}

func TestFuzz_FindingRendering_MultipleHeaders(t *testing.T) {
	wlFuzz := createWordlistWith(t, "admin.futurevera.thm")
	wlFoo := createWordlistWith(t, "secret123")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"-u", srv.URL,
		"-H", "Host: FUZZ",
		"-H", "Authorization: Bearer FOO",
		"-w", wlFuzz,
		"--foo", wlFoo,
		"-t", "1",
		"--no-progress",
	})

	if !strings.Contains(stdout, "  URL\n    "+srv.URL) {
		t.Errorf("expected URL block, got:\n%s", stdout)
	}
	if strings.Count(stdout, "\n  Header\n") != 1 {
		t.Errorf("expected exactly 1 Header section label, got count=%d in:\n%s", strings.Count(stdout, "\n  Header\n"), stdout)
	}
	if !strings.Contains(stdout, "    Host: admin.futurevera.thm") {
		t.Errorf("expected Host header under Header section, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "    Authorization: Bearer secret123") {
		t.Errorf("expected Authorization header under Header section, got:\n%s", stdout)
	}
}

func TestFuzz_FindingRendering_Cookie(t *testing.T) {
	wl := createWordlistWith(t, "sessionval123")
	var receivedCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := r.Cookie("session")
		if c != nil {
			receivedCookie = c.Value
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"-u", srv.URL,
		"-b", "session=FUZZ",
		"-w", wl,
		"-t", "1",
		"--no-progress",
	})

	if receivedCookie != "sessionval123" {
		t.Errorf("expected wire cookie session=sessionval123, got %q", receivedCookie)
	}

	if !strings.Contains(stdout, "  URL\n    "+srv.URL) {
		t.Errorf("expected URL block, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "  Cookie\n    session=sessionval123") {
		t.Errorf("expected Cookie block, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "Header") || strings.Contains(stdout, "Body") {
		t.Errorf("expected unchanged components not to be rendered, got:\n%s", stdout)
	}
}

func TestFuzz_FindingRendering_Body(t *testing.T) {
	wl := createWordlistWith(t, "admin")
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"-u", srv.URL,
		"-d", `{"username":"FUZZ"}`,
		"-w", wl,
		"-t", "1",
		"--no-progress",
	})

	if receivedBody != `{"username":"admin"}` {
		t.Errorf("expected wire body to be %q, got %q", `{"username":"admin"}`, receivedBody)
	}

	if !strings.Contains(stdout, "  URL\n    "+srv.URL) {
		t.Errorf("expected URL block, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "  Body\n    {\"username\":\"admin\"}") {
		t.Errorf("expected Body block, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "Cookie") || strings.Contains(stdout, "Header") {
		t.Errorf("expected unchanged components not to be rendered, got:\n%s", stdout)
	}
}

func TestFuzz_FindingRendering_MultipleLocations(t *testing.T) {
	wlFuzz := createWordlistWith(t, "mypath")
	wlFoo := createWordlistWith(t, "mytoken")
	wlBar := createWordlistWith(t, "mycookie")
	wlBaz := createWordlistWith(t, "mybody")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"-u", srv.URL + "/FUZZ",
		"-H", "X-Auth: FOO",
		"-b", "sess=BAR",
		"-d", "data=BAZ",
		"-w", wlFuzz,
		"--foo", wlFoo,
		"--bar", wlBar,
		"--baz", wlBaz,
		"-t", "1",
		"--no-progress",
	})

	if !strings.Contains(stdout, "  URL\n    "+srv.URL+"/mypath") {
		t.Errorf("expected URL block, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "  Header\n    X-Auth: mytoken") {
		t.Errorf("expected Header block, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "  Cookie\n    sess=mycookie") {
		t.Errorf("expected Cookie block, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "  Body\n    data=mybody") {
		t.Errorf("expected Body block, got:\n%s", stdout)
	}
}

func TestFuzz_FindingRendering_QuietMode(t *testing.T) {
	wl := createWordlistWith(t, "admin")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"-u", srv.URL + "/FUZZ",
		"-H", "X-Custom: FUZZ",
		"-w", wl,
		"-t", "1",
		"-q",
	})

	expected := srv.URL + "/admin\n"
	if stdout != expected {
		t.Errorf("expected quiet output %q, got %q", expected, stdout)
	}
}

func TestFuzz_FindingRendering_JSON(t *testing.T) {
	wl := createWordlistWith(t, "customval")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"-u", srv.URL,
		"-H", "X-Custom: FUZZ",
		"-w", wl,
		"-t", "1",
		"--format", "json",
		"--no-progress",
	})

	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON finding output: %v\nOutput was: %s", err, stdout)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 finding in JSON output, got %d", len(parsed))
	}
	fuzzFields, ok := parsed[0]["fuzz"].([]interface{})
	if !ok || len(fuzzFields) != 1 {
		t.Fatalf("expected fuzz array with 1 item, got: %v", parsed[0]["fuzz"])
	}
	field := fuzzFields[0].(map[string]interface{})
	if field["name"] != "X-Custom" || field["value"] != "customval" {
		t.Errorf("unexpected fuzz field in JSON: %v", field)
	}
}

func TestFuzz_FindingRendering_NDJSON(t *testing.T) {
	wl := createWordlistWith(t, "customval")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"-u", srv.URL,
		"-H", "X-Custom: FUZZ",
		"-w", wl,
		"-t", "1",
		"--format", "ndjson",
		"--no-progress",
	})

	var parsed map[string]interface{}
	line := strings.TrimSpace(stdout)
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		t.Fatalf("failed to unmarshal NDJSON line: %v\nOutput was: %s", err, stdout)
	}
	fuzzFields, ok := parsed["fuzz"].([]interface{})
	if !ok || len(fuzzFields) != 1 {
		t.Fatalf("expected fuzz array in NDJSON, got: %v", parsed["fuzz"])
	}
}

func TestFuzz_FindingRendering_CSV(t *testing.T) {
	wl := createWordlistWith(t, "admin")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"-u", srv.URL + "/FUZZ",
		"-w", wl,
		"-t", "1",
		"--format", "csv",
		"--no-progress",
	})

	if !strings.Contains(stdout, "url,status,length,depth") {
		t.Errorf("expected CSV header, got: %s", stdout)
	}
	if !strings.Contains(stdout, srv.URL+"/admin,200,2,0") {
		t.Errorf("expected CSV row for finding, got: %s", stdout)
	}
}

func TestFuzz_FindingRendering_Markdown(t *testing.T) {
	wl := createWordlistWith(t, "admin")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"-u", srv.URL + "/FUZZ",
		"-w", wl,
		"-t", "1",
		"--format", "markdown",
		"--no-progress",
	})

	if !strings.Contains(stdout, "| URL | Status | Length | Depth |") {
		t.Errorf("expected Markdown header, got: %s", stdout)
	}
	if !strings.Contains(stdout, "| "+srv.URL+"/admin | 200 | 2 | 0 |") {
		t.Errorf("expected Markdown row for finding, got: %s", stdout)
	}
}

func TestFuzz_FindingRendering_MultiLineBody(t *testing.T) {
	wl := createWordlistWith(t, "admin")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	bodyTemplate := "{\n  \"username\": \"FUZZ\"\n}"
	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"-u", srv.URL,
		"-d", bodyTemplate,
		"-w", wl,
		"-t", "1",
		"--no-progress",
	})

	expectedBodyBlock := "  Body\n    {\n      \"username\": \"admin\"\n    }"
	if !strings.Contains(stdout, expectedBodyBlock) {
		t.Errorf("expected multi-line body block:\n%s\ngot:\n%s", expectedBodyBlock, stdout)
	}
}

func TestFuzz_FindingRendering_RawRequest(t *testing.T) {
	wl := createWordlistWith(t, "testval")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	u := strings.TrimPrefix(srv.URL, "http://")
	reqContent := "POST /api HTTP/1.1\r\nHost: " + u + "\r\nX-Token: FUZZ\r\nContent-Length: 4\r\n\r\ntest"
	tmpReq := filepath.Join(t.TempDir(), "request.txt")
	if err := os.WriteFile(tmpReq, []byte(reqContent), 0600); err != nil {
		t.Fatalf("failed to write request file: %v", err)
	}

	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"--request", tmpReq,
		"-w", wl,
		"-t", "1",
		"--no-progress",
	})

	if !strings.Contains(stdout, "  Header\n    X-Token: testval") {
		t.Errorf("expected X-Token header in finding output for raw request fuzzing, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "Host:") {
		t.Errorf("expected non-fuzzed Host header not to be displayed, got:\n%s", stdout)
	}
}

func TestFuzz_FindingRendering_ConsecutiveFindings(t *testing.T) {
	wl := createWordlistWith(t, "user1", "user2")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	stdout := runFuzzCmdAndCaptureStdout(t, []string{
		"-u", srv.URL,
		"-H", "X-User: FUZZ",
		"-w", wl,
		"-t", "1",
		"--no-progress",
	})

	expectedFinding1 := "[+] 200 - 2 B\n  URL\n    " + srv.URL + "\n  Header\n    X-User: user1"
	expectedFinding2 := "[+] 200 - 2 B\n  URL\n    " + srv.URL + "\n  Header\n    X-User: user2"

	if !strings.Contains(stdout, expectedFinding1) {
		t.Errorf("expected finding 1:\n%s\ngot:\n%s", expectedFinding1, stdout)
	}
	if !strings.Contains(stdout, expectedFinding2) {
		t.Errorf("expected finding 2:\n%s\ngot:\n%s", expectedFinding2, stdout)
	}

	expectedCombined := expectedFinding1 + "\n\n" + expectedFinding2 + "\n\n"
	if stdout != expectedCombined {
		t.Errorf("expected consecutive findings with single blank line between them:\n%q\ngot:\n%q", expectedCombined, stdout)
	}
}
