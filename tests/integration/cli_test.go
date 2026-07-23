package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	// Build the binary
	dir, err := os.Getwd()
	if err != nil {
		fmt.Println("failed to get wd")
		os.Exit(1)
	}

	// We are running in tests/integration
	projectRoot := filepath.Dir(filepath.Dir(dir))

	binName := "searchit-test-bin"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath = filepath.Join(projectRoot, binName)

	cmd := exec.Command("go", "build", "-o", binPath, projectRoot)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("failed to build binary: %v\n%s\n", err, out)
		os.Exit(1)
	}

	code := m.Run()

	os.Remove(binPath)
	os.Exit(code)
}

func setupMockServer(releasesBody string, releasesStatus int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(releasesStatus)
		fmt.Fprint(w, releasesBody)
	}))
}

func verifyGolden(t *testing.T, actual, goldenFile string) {
	goldenPath := filepath.Join("..", "..", "testdata", goldenFile)
	actual = strings.ReplaceAll(actual, "\r\n", "\n")

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		os.MkdirAll(filepath.Dir(goldenPath), 0755)
		os.WriteFile(goldenPath, []byte(actual), 0644)
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}
	expectedStr := strings.ReplaceAll(string(expected), "\r\n", "\n")

	if expectedStr != actual {
		t.Errorf("output does not match golden file %s.\nExpected:\n%s\nGot:\n%s\n", goldenPath, expectedStr, actual)
	}
}

func TestUpdateCheck(t *testing.T) {
	releasesJSON := `[{"tag_name": "v1.0.0", "draft": false}]`
	server := setupMockServer(releasesJSON, 200)
	defer server.Close()

	// Need to mock NEWS directory as well to be predictable
	// but the binary will read from actual project NEWS by default
	// Let's set the CWD to a temp dir so we can mock NEWS

	tempDir := t.TempDir()
	newsDir := filepath.Join(tempDir, "NEWS")
	os.Mkdir(newsDir, 0755)
	os.WriteFile(filepath.Join(newsDir, "v1.0.0.md"), []byte("This is news for v1.0.0\n"), 0644)

	env := map[string]string{
		"SEARCHIT_API_BASE": server.URL,
	}

	cmd := exec.Command(binPath, "update", "--check")
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Dir = tempDir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v\nOutput: %s", err, out)
	}

	verifyGolden(t, string(out), "update/update_check.golden")
}

func TestDoctorCommand(t *testing.T) {
	releasesJSON := `[{"tag_name": "v1.0.0", "draft": false}]`
	server := setupMockServer(releasesJSON, 200)
	defer server.Close()

	tempDir := t.TempDir()

	env := map[string]string{
		"SEARCHIT_API_BASE": server.URL,
	}

	cmd := exec.Command(binPath, "doctor")
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Dir = tempDir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v\nOutput: %s", err, out)
	}

	verifyGolden(t, string(out), "doctor/healthy.golden")
}

func TestNewsCommand(t *testing.T) {
	tempDir := t.TempDir()
	newsDir := filepath.Join(tempDir, "NEWS")
	os.Mkdir(newsDir, 0755)
	os.WriteFile(filepath.Join(newsDir, "v0.5.0.md"), []byte("# RELEASE v0.5.0\n\n### NEW\n- Feature A\n\n### FIXED\n- Bug B\n"), 0644)

	cmd := exec.Command(binPath, "news", "v0.5.0")
	cmd.Dir = tempDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v\nOutput: %s", err, out)
	}

	verifyGolden(t, string(out), "news/news.golden")
}

func TestRollbackCommand(t *testing.T) {
	releasesJSON := `[{"tag_name": "v1.0.0", "draft": false}, {"tag_name": "v0.4.0", "draft": false}]`
	server := setupMockServer(releasesJSON, 200)
	defer server.Close()

	tempDir := t.TempDir()
	env := map[string]string{
		"SEARCHIT_API_BASE": server.URL,
	}

	cmd := exec.Command(binPath, "update", "--rollback", "v0.4.0", "--dry-run")
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Dir = tempDir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v\nOutput: %s", err, out)
	}

	verifyGolden(t, string(out), "update/rollback.golden")
}

func TestDowngradeCommand(t *testing.T) {
	releasesJSON := `[{"tag_name": "v1.0.0", "draft": false}, {"tag_name": "v0.4.0", "draft": false}]`
	server := setupMockServer(releasesJSON, 200)
	defer server.Close()

	tempDir := t.TempDir()
	env := map[string]string{
		"SEARCHIT_API_BASE": server.URL,
	}

	// current version is v0.5.0
	cmd := exec.Command(binPath, "update", "--install", "v0.4.0", "--dry-run")
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Dir = tempDir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v\nOutput: %s", err, out)
	}

	verifyGolden(t, string(out), "update/downgrade.golden")
}
