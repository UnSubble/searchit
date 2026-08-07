package cmd_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unsubble/searchit/cmd"
)

func TestFuzz_RichPlaceholderConfiguration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer srv.Close()

	tempDir := t.TempDir()
	fooFile := filepath.Join(tempDir, "foo.txt")
	_ = os.WriteFile(fooFile, []byte("token1\ntoken2\ntoken3\n"), 0644) // 3 entries

	barFile := filepath.Join(tempDir, "bar.txt")
	_ = os.WriteFile(barFile, []byte("val1\nval2\n"), 0644) // 2 entries

	// We'll capture stderr where the configuration is printed
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	fuzzCmd, _ := cmd.NewFuzzCmd()
	fuzzCmd.SetArgs([]string{
		"-u", srv.URL + "/FUZZ?search=FOO",
		"-H", "Host: BUZZ.example.com",
		"-H", "Authorization: Bearer FOO",
		"-d", `{"item":"BAZ"}`,
		"-b", "session=BAR",
		"--foo", fooFile,
		"--bar", barFile,
		"--baz", "=bar",
		"--buzz", "=fuzz",
		"-t", "1",
		"--no-progress",
	})

	cxx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := fuzzCmd.ExecuteContext(cxx)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()

	if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1. Verify Placeholders section exists
	if !strings.Contains(out, "Placeholders") {
		t.Fatalf("expected 'Placeholders' section in output, got:\n%s", out)
	}

	// 2. Verify FUZZ details:
	// Location: URL
	// Source: embedded
	// Entries: 4751
	if !strings.Contains(out, "  FUZZ") {
		t.Errorf("missing '  FUZZ' block in output:\n%s", out)
	}
	if !strings.Contains(out, "Location      URL") {
		t.Errorf("FUZZ location mismatch in output:\n%s", out)
	}
	if !strings.Contains(out, "Source        embedded") {
		t.Errorf("FUZZ source mismatch in output:\n%s", out)
	}
	if !strings.Contains(out, "Entries       4751") {
		t.Errorf("FUZZ entries mismatch in output:\n%s", out)
	}

	// 3. Verify FOO details:
	// Location: Query parameter, Header: Authorization
	// Source: fooFile
	// Entries: 3
	if !strings.Contains(out, "  FOO") {
		t.Errorf("missing '  FOO' block in output:\n%s", out)
	}
	if !strings.Contains(out, "Query parameter") || !strings.Contains(out, "Header: Authorization") {
		t.Errorf("FOO location mismatch (expected Query parameter and Header: Authorization) in output:\n%s", out)
	}
	if !strings.Contains(out, fooFile) {
		t.Errorf("FOO source file mismatch in output:\n%s", out)
	}
	if !strings.Contains(out, "Entries       3") {
		t.Errorf("FOO entries mismatch (expected 3) in output:\n%s", out)
	}

	// 4. Verify BAR details:
	// Location: Cookie
	// Source: barFile
	// Entries: 2
	if !strings.Contains(out, "  BAR") {
		t.Errorf("missing '  BAR' block in output:\n%s", out)
	}
	if !strings.Contains(out, "Location      Cookie") {
		t.Errorf("BAR location mismatch in output:\n%s", out)
	}
	if !strings.Contains(out, barFile) {
		t.Errorf("BAR source file mismatch in output:\n%s", out)
	}
	if !strings.Contains(out, "Entries       2") {
		t.Errorf("BAR entries mismatch (expected 2) in output:\n%s", out)
	}

	// 5. Verify BAZ details:
	// Location: Body
	// Source: alias (=bar)
	// Entries: 2
	if !strings.Contains(out, "  BAZ") {
		t.Errorf("missing '  BAZ' block in output:\n%s", out)
	}
	if !strings.Contains(out, "Location      Body") {
		t.Errorf("BAZ location mismatch in output:\n%s", out)
	}
	if !strings.Contains(out, "Source        alias (=bar)") {
		t.Errorf("BAZ source mismatch (expected alias (=bar)) in output:\n%s", out)
	}

	// 6. Verify BUZZ details:
	// Location: Header: Host
	// Source: alias (=fuzz)
	// Entries: 4751
	if !strings.Contains(out, "  BUZZ") {
		t.Errorf("missing '  BUZZ' block in output:\n%s", out)
	}
	if !strings.Contains(out, "Location      Header: Host") {
		t.Errorf("BUZZ location mismatch in output:\n%s", out)
	}
	if !strings.Contains(out, "Source        alias (=fuzz)") {
		t.Errorf("BUZZ source mismatch (expected alias (=fuzz)) in output:\n%s", out)
	}

	// 7. Verify Stable Ordering: FUZZ -> FOO -> BAR -> BAZ -> BUZZ
	idxFUZZ := strings.Index(out, "  FUZZ\n")
	idxFOO := strings.Index(out, "  FOO\n")
	idxBAR := strings.Index(out, "  BAR\n")
	idxBAZ := strings.Index(out, "  BAZ\n")
	idxBUZZ := strings.Index(out, "  BUZZ\n")

	if idxFUZZ == -1 || idxFOO == -1 || idxBAR == -1 || idxBAZ == -1 || idxBUZZ == -1 {
		t.Fatalf("one or more placeholder blocks not found in output:\n%s", out)
	}

	if !(idxFUZZ < idxFOO && idxFOO < idxBAR && idxBAR < idxBAZ && idxBAZ < idxBUZZ) {
		t.Errorf("placeholders not in stable canonical order (FUZZ < FOO < BAR < BAZ < BUZZ): FUZZ=%d, FOO=%d, BAR=%d, BAZ=%d, BUZZ=%d",
			idxFUZZ, idxFOO, idxBAR, idxBAZ, idxBUZZ)
	}
}

func TestFuzz_RichPlaceholderMultipleLocations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	fuzzCmd, _ := cmd.NewFuzzCmd()
	fuzzCmd.SetArgs([]string{
		"-u", srv.URL + "/FUZZ",
		"-H", "Host: FUZZ.example.com",
		"-d", "data=FUZZ",
		"-t", "1",
		"--no-progress",
	})

	cxx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = fuzzCmd.ExecuteContext(cxx)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()

	// Verify multiple locations for FUZZ: URL, Header: Host, Body
	if !strings.Contains(out, "Location      URL, Header: Host, Body") {
		t.Errorf("expected combined location 'URL, Header: Host, Body', got output:\n%s", out)
	}
}
