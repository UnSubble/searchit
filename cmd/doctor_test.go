package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/unsubble/searchit/internal/news"
	"github.com/unsubble/searchit/internal/version"
)

// setupMockGitHub creates a mock server for GitHub releases API.
func setupMockGitHub(t *testing.T, body string, status int) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func getGoBinDir() string {
	if goBin, err := exec.LookPath("go"); err == nil {
		return filepath.Dir(goBin)
	}
	if gr := runtime.GOROOT(); gr != "" {
		return filepath.Join(gr, "bin")
	}
	return ""
}

func executeDoctorCmd(t *testing.T) (string, error) {
	t.Helper()

	rootCmd.SetContext(context.Background())
	doctorCmd.SetContext(context.Background())
	rootCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	doctorCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	rootCmd.SetArgs([]string{"doctor"})

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)

	err := rootCmd.ExecuteContext(context.Background())
	return buf.String(), err
}

func TestDoctorCmd_OutputFormat(t *testing.T) {
	mockURL := setupMockGitHub(t, `[{"tag_name": "v0.7.0", "draft": false}]`, 200)
	t.Setenv("SEARCHIT_API_BASE", mockURL)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", getGoBinDir())
	defer os.Setenv("PATH", origPath)

	out, _ := executeDoctorCmd(t)

	// Header
	if !strings.Contains(out, "searchit doctor") {
		t.Errorf("expected header 'searchit doctor', got:\n%s", out)
	}

	// Separator
	if !strings.Contains(out, "----------------------------------------") {
		t.Errorf("expected 40-character separator, got:\n%s", out)
	}

	// Check that check names are rendered
	expectedChecks := []string{
		"VERSION",
		"GO VERSION",
		"CONFIGURATION",
		"NEWS SYSTEM",
		"GITHUB CONNECTIVITY",
		"UPDATE SYSTEM",
		"MULTIPLE BINARIES",
		"ACTIVE EXECUTABLE",
	}
	for _, check := range expectedChecks {
		if !strings.Contains(out, check) {
			t.Errorf("expected output to contain check %q, got:\n%s", check, out)
		}
	}

	// Check status line
	if !strings.Contains(out, "STATUS:") {
		t.Errorf("expected output to contain 'STATUS:', got:\n%s", out)
	}
}

func TestDoctorCmd_ExitZeroOnHealthy(t *testing.T) {
	mockURL := setupMockGitHub(t, `[{"tag_name": "v0.7.0", "draft": false}]`, 200)
	t.Setenv("SEARCHIT_API_BASE", mockURL)

	// Ensure news file is found for current version.
	tmpDir := t.TempDir()
	origNewsDir := news.GetNewsDir()
	news.SetNewsDir(tmpDir)
	defer news.SetNewsDir(origNewsDir)

	if err := os.WriteFile(filepath.Join(tmpDir, version.Version+".md"), []byte("# Notes"), 0o644); err != nil {
		t.Fatalf("failed to create news file: %v", err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", getGoBinDir())
	defer os.Setenv("PATH", origPath)

	out, err := executeDoctorCmd(t)
	if err != nil {
		t.Fatalf("expected doctor to succeed (exit 0), got error: %v\nOutput:\n%s", err, out)
	}

	if !strings.Contains(out, "STATUS: HEALTHY\n") {
		t.Errorf("expected 'STATUS: HEALTHY', got:\n%s", out)
	}
}

func TestDoctorCmd_ExitZeroOnWarningsOnly(t *testing.T) {
	mockURL := setupMockGitHub(t, `[{"tag_name": "v0.7.0", "draft": false}]`, 200)
	t.Setenv("SEARCHIT_API_BASE", mockURL)

	// News file does NOT exist → triggers WARN on NEWS SYSTEM.
	origNewsDir := news.GetNewsDir()
	news.SetNewsDir("/nonexistent/news/path")
	defer news.SetNewsDir(origNewsDir)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", getGoBinDir())
	defer os.Setenv("PATH", origPath)

	out, err := executeDoctorCmd(t)
	if err != nil {
		t.Fatalf("warnings only must NOT cause non-zero exit, got error: %v\nOutput:\n%s", err, out)
	}

	if !strings.Contains(out, "STATUS: HEALTHY WITH WARNINGS") {
		t.Errorf("expected 'STATUS: HEALTHY WITH WARNINGS', got:\n%s", out)
	}
}

func TestDoctorCmd_NonZeroExitOnFail(t *testing.T) {
	// Simulate GitHub failure → causes GITHUB CONNECTIVITY=FAIL and UPDATE SYSTEM=FAIL.
	mockURL := setupMockGitHub(t, `{}`, 500)
	t.Setenv("SEARCHIT_API_BASE", mockURL)

	out, err := executeDoctorCmd(t)
	if err == nil {
		t.Fatalf("expected doctor to return an error (non-zero exit) when checks fail, but err was nil\nOutput:\n%s", out)
	}

	if !strings.Contains(out, "STATUS: NOT READY") {
		t.Errorf("expected 'STATUS: NOT READY', got:\n%s", out)
	}
	if !strings.Contains(out, "FAIL  GITHUB CONNECTIVITY") {
		t.Errorf("expected GITHUB CONNECTIVITY to be marked FAIL, got:\n%s", out)
	}
}

func TestDoctorCmd_DiagnosticMessagesPresent(t *testing.T) {
	mockURL := setupMockGitHub(t, `[{"tag_name": "v0.7.0", "draft": false}]`, 200)
	t.Setenv("SEARCHIT_API_BASE", mockURL)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", getGoBinDir())
	defer os.Setenv("PATH", origPath)

	out, _ := executeDoctorCmd(t)

	// The current version should be displayed under the VERSION check.
	if !strings.Contains(out, version.Version) {
		t.Errorf("expected output to contain current version %q, got:\n%s", version.Version, out)
	}
}
