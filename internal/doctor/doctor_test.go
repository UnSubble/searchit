package doctor

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/github"
	"github.com/unsubble/searchit/internal/news"
	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/testutil/command"
	"github.com/unsubble/searchit/internal/version"
)

// TestHelperProcess enables MockExecutor support.
func TestHelperProcess(t *testing.T) {
	command.HandleHelperProcess()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mockTransport redirects api.github.com requests to a local test server.
type mockTransport struct {
	serverURL string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "api.github.com" {
		req.URL.Scheme = "http"
		req.URL.Host = m.serverURL[7:] // strip "http://"
	}
	transport := http.DefaultTransport
	if transport == nil || transport == m {
		transport = new(http.Transport)
	}
	return transport.RoundTrip(req)
}

type mockFailingProfileStore struct{}

func (m *mockFailingProfileStore) Load(_ string) (*profile.Profile, error) {
	return nil, fmt.Errorf("corrupted profile")
}
func (m *mockFailingProfileStore) List() ([]profile.ProfileInfo, error) {
	return nil, fmt.Errorf("corrupted profile store")
}
func (m *mockFailingProfileStore) LoadRaw(_ string) ([]byte, error) {
	return nil, fmt.Errorf("corrupted profile")
}
func (m *mockFailingProfileStore) Create(_ profile.Profile) error {
	return fmt.Errorf("read only")
}

// setupGitHubServer creates a mock GitHub releases API server.
func setupGitHubServer(body string, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
}

// setupGitHubClient returns a github.Client pointed at the mock server and the
// test-scoped transport installed on http.DefaultTransport.
func setupGitHubClient(t *testing.T, body string, status int) (*github.Client, func()) {
	t.Helper()
	server := setupGitHubServer(body, status)
	originalTransport := http.DefaultTransport
	http.DefaultTransport = &mockTransport{serverURL: server.URL}
	client := github.NewClient()

	cleanup := func() {
		http.DefaultTransport = originalTransport
		server.Close()
	}
	return client, cleanup
}

// withVersionOverride temporarily changes version.Version in a test.
func withVersionOverride(t *testing.T, v string) {
	t.Helper()
	orig := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = orig })
}

// withNewsDir temporarily sets the news directory.
func withNewsDir(t *testing.T, dir string) {
	t.Helper()
	orig := news.GetNewsDir()
	news.SetNewsDir(dir)
	t.Cleanup(func() { news.SetNewsDir(orig) })
}

// makeNewsFile writes a news file for version v in dir and sets the news dir.
func makeNewsFile(t *testing.T, dir, v string) {
	t.Helper()
	f := filepath.Join(dir, v+".md")
	if err := os.WriteFile(f, []byte("# Release notes"), 0o644); err != nil {
		t.Fatalf("could not create news file: %v", err)
	}
}

// resultsMap converts a Summary's Results slice into a map keyed by Name.
func resultsMap(s Summary) map[string]CheckResult {
	m := make(map[string]CheckResult, len(s.Results))
	for _, r := range s.Results {
		m[r.Name] = r
	}
	return m
}

// assertStatus fails the test if check Name does not have the expected status.
func assertStatus(t *testing.T, m map[string]CheckResult, name, want string) {
	t.Helper()
	r, ok := m[name]
	if !ok {
		t.Errorf("check %q not found in results", name)
		return
	}
	if r.Status != want {
		t.Errorf("check %q: got status %q, want %q (message: %q)", name, r.Status, want, r.Message)
	}
}

// assertMessageContains fails the test if the check's Message does not contain substr.
func assertMessageContains(t *testing.T, m map[string]CheckResult, name, substr string) {
	t.Helper()
	r, ok := m[name]
	if !ok {
		t.Errorf("check %q not found in results", name)
		return
	}
	if !strings.Contains(r.Message, substr) {
		t.Errorf("check %q message %q does not contain %q", name, r.Message, substr)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRunAllChecks_HealthySystem(t *testing.T) {
	ghBody := `[{"tag_name": "v0.7.0", "draft": false}]`
	ghClient, cleanup := setupGitHubClient(t, ghBody, 200)
	defer cleanup()

	// Use current (dev) version — semver parse must pass.
	// Ensure news dir is empty so news check produces WARN, not FAIL.
	tmpDir := t.TempDir()
	makeNewsFile(t, tmpDir, version.Version)
	withNewsDir(t, tmpDir)

	doc := &Doctor{
		Executor:     &command.MockExecutor{MockOutput: "go version go1.23.0 linux/amd64\n", ExitCode: 0},
		ProfileStore: profile.NewStore(),
		GitHubClient: ghClient,
		NewsVersion:  version.Version,
	}

	// Clear PATH so MULTIPLE BINARIES check doesn't pick up stray binaries.
	orig := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", orig)

	summary := doc.RunAllChecks()
	m := resultsMap(summary)

	assertStatus(t, m, "VERSION", StatusPass)
	assertStatus(t, m, "GO VERSION", StatusPass)
	assertStatus(t, m, "CONFIGURATION", StatusPass)
	assertStatus(t, m, "NEWS SYSTEM", StatusPass)
	assertStatus(t, m, "GITHUB CONNECTIVITY", StatusPass)
	assertStatus(t, m, "UPDATE SYSTEM", StatusPass)
	assertStatus(t, m, "MULTIPLE BINARIES", StatusPass)

	if summary.HasFail {
		t.Errorf("expected HasFail=false for healthy system; checks: %+v", m)
	}
}

func TestRunAllChecks_VersionInvalid(t *testing.T) {
	withVersionOverride(t, "not-a-valid-semver")

	ghBody := `[{"tag_name": "v0.7.0", "draft": false}]`
	ghClient, cleanup := setupGitHubClient(t, ghBody, 200)
	defer cleanup()

	doc := &Doctor{
		Executor:     &command.MockExecutor{MockOutput: "go version go1.23.0\n", ExitCode: 0},
		ProfileStore: profile.NewStore(),
		GitHubClient: ghClient,
		NewsVersion:  "v0.7.0",
	}
	summary := doc.RunAllChecks()
	m := resultsMap(summary)

	assertStatus(t, m, "VERSION", StatusFail)
	assertMessageContains(t, m, "VERSION", "not-a-valid-semver")
	if !summary.HasFail {
		t.Error("expected HasFail=true when VERSION fails")
	}
}

func TestRunAllChecks_GitHubConnectivityFails(t *testing.T) {
	// 500 response causes FetchVersions to fail.
	ghClient, cleanup := setupGitHubClient(t, `{}`, 500)
	defer cleanup()

	doc := &Doctor{
		Executor:     &command.MockExecutor{MockOutput: "go version go1.23.0\n", ExitCode: 0},
		ProfileStore: profile.NewStore(),
		GitHubClient: ghClient,
		NewsVersion:  "v0.7.0",
	}
	summary := doc.RunAllChecks()
	m := resultsMap(summary)

	assertStatus(t, m, "GITHUB CONNECTIVITY", StatusFail)
	assertStatus(t, m, "UPDATE SYSTEM", StatusFail)
	assertMessageContains(t, m, "UPDATE SYSTEM", "GitHub connectivity failed")
	if !summary.HasFail {
		t.Error("expected HasFail=true when GitHub is unreachable")
	}
}

func TestRunAllChecks_GoVersionNotVerified(t *testing.T) {
	// exit code 1 → go is not found.
	ghBody := `[{"tag_name": "v0.7.0", "draft": false}]`
	ghClient, cleanup := setupGitHubClient(t, ghBody, 200)
	defer cleanup()

	doc := &Doctor{
		Executor:     &command.MockExecutor{MockOutput: "command not found: go", ExitCode: 1},
		ProfileStore: profile.NewStore(),
		GitHubClient: ghClient,
		NewsVersion:  "v0.7.0",
	}
	summary := doc.RunAllChecks()
	m := resultsMap(summary)

	assertStatus(t, m, "GO VERSION", StatusNotVerified)
	// NOT VERIFIED must NOT set HasFail.
	// (HasFail could be true from other checks, so we test only that GO VERSION is not FAIL.)
	if m["GO VERSION"].Status == StatusFail {
		t.Error("GO VERSION must be NOT VERIFIED, not FAIL, when go is absent")
	}
	if !summary.HasUnverified {
		t.Error("expected HasUnverified=true when GO VERSION is NOT VERIFIED")
	}
}

func TestRunAllChecks_ConfigurationBroken(t *testing.T) {
	ghBody := `[{"tag_name": "v0.7.0", "draft": false}]`
	ghClient, cleanup := setupGitHubClient(t, ghBody, 200)
	defer cleanup()

	doc := &Doctor{
		Executor:     &command.MockExecutor{MockOutput: "go version go1.23.0\n", ExitCode: 0},
		ProfileStore: &mockFailingProfileStore{},
		GitHubClient: ghClient,
		NewsVersion:  "v0.7.0",
	}
	summary := doc.RunAllChecks()
	m := resultsMap(summary)

	assertStatus(t, m, "CONFIGURATION", StatusFail)
	assertMessageContains(t, m, "CONFIGURATION", "corrupted profile store")
	if !summary.HasFail {
		t.Error("expected HasFail=true when CONFIGURATION fails")
	}
}

func TestRunAllChecks_NewsUnavailable(t *testing.T) {
	// Set news dir to a path that doesn't exist — news.Fetch returns "no news available".
	withNewsDir(t, "/nonexistent/path/nowhere")

	ghBody := `[{"tag_name": "v0.7.0", "draft": false}]`
	ghClient, cleanup := setupGitHubClient(t, ghBody, 200)
	defer cleanup()

	doc := &Doctor{
		Executor:     &command.MockExecutor{MockOutput: "go version go1.23.0\n", ExitCode: 0},
		ProfileStore: profile.NewStore(),
		GitHubClient: ghClient,
		NewsVersion:  "v99.99.99",
	}
	summary := doc.RunAllChecks()
	m := resultsMap(summary)

	// Missing news → WARN not FAIL.
	assertStatus(t, m, "NEWS SYSTEM", StatusWarn)
	if m["NEWS SYSTEM"].Status == StatusFail {
		t.Error("NEWS SYSTEM must be WARN (not FAIL) when news file is missing")
	}
	// Warnings alone must NOT set HasFail.
	// (There may be a FAIL from other checks; test only the specific news semantics.)
}

func TestRunAllChecks_NewsFound(t *testing.T) {
	tmpDir := t.TempDir()
	const testVersion = "v0.7.0"
	makeNewsFile(t, tmpDir, testVersion)
	withNewsDir(t, tmpDir)

	ghBody := `[{"tag_name": "v0.7.0", "draft": false}]`
	ghClient, cleanup := setupGitHubClient(t, ghBody, 200)
	defer cleanup()

	doc := &Doctor{
		Executor:     &command.MockExecutor{MockOutput: "go version go1.23.0\n", ExitCode: 0},
		ProfileStore: profile.NewStore(),
		GitHubClient: ghClient,
		NewsVersion:  testVersion,
	}
	summary := doc.RunAllChecks()
	m := resultsMap(summary)

	assertStatus(t, m, "NEWS SYSTEM", StatusPass)
	assertMessageContains(t, m, "NEWS SYSTEM", testVersion)
}

func TestRunAllChecks_MultipleBinariesWarn(t *testing.T) {
	// Create two directories each containing a fake "searchit" binary.
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	bin1 := filepath.Join(dir1, "searchit")
	bin2 := filepath.Join(dir2, "searchit")
	for _, p := range []string{bin1, bin2} {
		if err := os.WriteFile(p, []byte("fake"), 0o755); err != nil {
			t.Fatalf("could not create fake binary: %v", err)
		}
	}

	ghBody := `[{"tag_name": "v0.7.0", "draft": false}]`
	ghClient, cleanup := setupGitHubClient(t, ghBody, 200)
	defer cleanup()

	orig := os.Getenv("PATH")
	os.Setenv("PATH", dir1+string(os.PathListSeparator)+dir2)
	defer os.Setenv("PATH", orig)

	doc := &Doctor{
		// Use DefaultExecutor because CheckMultipleInstallations uses os.Stat, not executor.
		Executor:     command.DefaultExecutor{},
		ProfileStore: profile.NewStore(),
		GitHubClient: ghClient,
		NewsVersion:  "v0.7.0",
	}
	summary := doc.RunAllChecks()
	m := resultsMap(summary)

	assertStatus(t, m, "MULTIPLE BINARIES", StatusWarn)
	assertMessageContains(t, m, "MULTIPLE BINARIES", "2 searchit executables")
	if !summary.HasWarn {
		t.Error("expected HasWarn=true for multiple binaries")
	}
}

func TestRunAllChecks_UpdateSystemAhead(t *testing.T) {
	// Current version is "ahead" of the latest stable returned by mock.
	withVersionOverride(t, "v1.0.0")

	ghBody := `[{"tag_name": "v0.7.0", "draft": false}]`
	ghClient, cleanup := setupGitHubClient(t, ghBody, 200)
	defer cleanup()

	doc := &Doctor{
		Executor:     &command.MockExecutor{MockOutput: "go version go1.23.0\n", ExitCode: 0},
		ProfileStore: profile.NewStore(),
		GitHubClient: ghClient,
		NewsVersion:  "v0.7.0",
	}
	summary := doc.RunAllChecks()
	m := resultsMap(summary)

	assertStatus(t, m, "UPDATE SYSTEM", StatusPass)
	assertMessageContains(t, m, "UPDATE SYSTEM", "ahead of latest stable")
}

func TestRunAllChecks_UpdateSystemUpdateAvailable(t *testing.T) {
	// Current version is BEHIND the latest stable.
	withVersionOverride(t, "v0.6.0")

	ghBody := `[{"tag_name": "v0.7.0", "draft": false}]`
	ghClient, cleanup := setupGitHubClient(t, ghBody, 200)
	defer cleanup()

	doc := &Doctor{
		Executor:     &command.MockExecutor{MockOutput: "go version go1.23.0\n", ExitCode: 0},
		ProfileStore: profile.NewStore(),
		GitHubClient: ghClient,
		NewsVersion:  "v0.7.0",
	}
	summary := doc.RunAllChecks()
	m := resultsMap(summary)

	assertStatus(t, m, "UPDATE SYSTEM", StatusPass)
	assertMessageContains(t, m, "UPDATE SYSTEM", "update available")
	assertMessageContains(t, m, "UPDATE SYSTEM", "v0.7.0")
}

func TestRunAllChecks_WarningsOnlyDoNotSetHasFail(t *testing.T) {
	// Set up conditions that trigger WARN but no FAIL:
	// - NEWS: unavailable → WARN
	// - MULTIPLE BINARIES: none → PASS (PATH="")
	// - GitHub OK
	withNewsDir(t, "/nonexistent/path/nowhere")

	ghBody := `[{"tag_name": "v0.7.0", "draft": false}]`
	ghClient, cleanup := setupGitHubClient(t, ghBody, 200)
	defer cleanup()

	orig := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", orig)

	doc := &Doctor{
		Executor:     &command.MockExecutor{MockOutput: "go version go1.23.0\n", ExitCode: 0},
		ProfileStore: profile.NewStore(),
		GitHubClient: ghClient,
		NewsVersion:  "v99.99.99",
	}
	summary := doc.RunAllChecks()

	if summary.HasFail {
		m := resultsMap(summary)
		for name, r := range m {
			if r.Status == StatusFail {
				t.Errorf("unexpected FAIL in %q: %s", name, r.Message)
			}
		}
		t.Error("warnings only must NOT set HasFail=true")
	}
	if !summary.HasWarn {
		t.Error("expected HasWarn=true (NEWS SYSTEM should be WARN)")
	}
}

func TestCheckResult_MessageAlwaysSet(t *testing.T) {
	// Verify that every check produces a non-empty Message regardless of outcome.
	ghBody := `[{"tag_name": "v0.7.0", "draft": false}]`
	ghClient, cleanup := setupGitHubClient(t, ghBody, 200)
	defer cleanup()

	orig := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", orig)

	doc := &Doctor{
		Executor:     &command.MockExecutor{MockOutput: "go version go1.23.0\n", ExitCode: 0},
		ProfileStore: profile.NewStore(),
		GitHubClient: ghClient,
		NewsVersion:  "v0.7.0",
	}
	summary := doc.RunAllChecks()

	for _, r := range summary.Results {
		if strings.TrimSpace(r.Message) == "" {
			t.Errorf("check %q has an empty Message", r.Name)
		}
	}
}
