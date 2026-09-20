package doctor

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/unsubble/searchit/internal/news"
	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/testutil/command"
	"github.com/unsubble/searchit/internal/version"
)

func TestHelperProcess(t *testing.T) {
	command.HandleHelperProcess()
}

func setupMockServer(releasesBody string, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, releasesBody)
	}))
}

type mockFailingProfileStore struct{}

func (m *mockFailingProfileStore) Load(name string) (*profile.Profile, error) {
	return nil, fmt.Errorf("corrupted profile")
}
func (m *mockFailingProfileStore) List() ([]profile.ProfileInfo, error) {
	return nil, fmt.Errorf("corrupted profile store")
}
func (m *mockFailingProfileStore) LoadRaw(name string) ([]byte, error) {
	return nil, fmt.Errorf("corrupted profile")
}
func (m *mockFailingProfileStore) Create(p profile.Profile) error {
	return fmt.Errorf("read only")
}

func TestRunAllChecks(t *testing.T) {
	tests := []struct {
		name            string
		ghBody          string
		ghStatus        int
		execExit        int
		execOutput      string
		versionOverride string
		profileStore    profile.Store
		newsDirOverride string
		wantStatus      bool // true = HEALTHY, false = NOT READY
		checkHas        map[string]string
	}{
		{
			name:       "healthy_system",
			ghBody:     `[{"tag_name": "v1.0.0", "draft": false}]`,
			ghStatus:   200,
			wantStatus: false, // INSTALLATION METHOD will be WARNING
			checkHas: map[string]string{
				"VERSION":             "PASS",
				"UPDATE SYSTEM":       "PASS",
				"NEWS SYSTEM":         "PASS",
				"CONFIGURATION":       "PASS",
				"GITHUB CONNECTIVITY": "PASS",
				"RELEASE CHANNEL":     "PASS",
				"GO VERSION":          "PASS",
				"ACTIVE EXECUTABLE":   "PASS",
				"MULTIPLE BINARIES":   "PASS",
				"INSTALLATION METHOD": "WARNING", // mock will fail to detect gobin
			},
		},
		{
			name:       "github_connectivity_fails",
			ghBody:     `{}`,
			ghStatus:   500,
			wantStatus: false,
			checkHas: map[string]string{
				"UPDATE SYSTEM":       "MAY BE INVALID",
				"GITHUB CONNECTIVITY": "FAIL",
				"GO VERSION":          "PASS",
				"ACTIVE EXECUTABLE":   "PASS",
				"MULTIPLE BINARIES":   "PASS",
				"INSTALLATION METHOD": "WARNING",
			},
		},
		{
			name:       "go_version_not_verified_when_executor_fails",
			ghBody:     `[{"tag_name": "v1.0.0", "draft": false}]`,
			ghStatus:   200,
			execExit:   1,
			execOutput: "command not found: go",
			wantStatus: false,
			checkHas: map[string]string{
				"GO VERSION": "NOT VERIFIED",
			},
		},
		{
			name:            "version_invalid_fails",
			ghBody:          `[{"tag_name": "v1.0.0", "draft": false}]`,
			ghStatus:        200,
			versionOverride: "invalid-version",
			wantStatus:      false,
			checkHas: map[string]string{
				"VERSION":         "FAIL",
				"RELEASE CHANNEL": "FAIL",
			},
		},
		{
			name:         "configuration_fails_on_broken_store",
			ghBody:       `[{"tag_name": "v1.0.0", "draft": false}]`,
			ghStatus:     200,
			profileStore: &mockFailingProfileStore{},
			wantStatus:   false,
			checkHas: map[string]string{
				"CONFIGURATION": "FAIL",
			},
		},
		{
			name:            "news_system_fails_when_news_dir_unset",
			ghBody:          `[{"tag_name": "v1.0.0", "draft": false}]`,
			ghStatus:        200,
			newsDirOverride: "",
			wantStatus:      false,
			checkHas: map[string]string{
				"NEWS SYSTEM": "FAIL",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.versionOverride != "" {
				origV := version.Version
				version.Version = tt.versionOverride
				defer func() { version.Version = origV }()
			}

			if tt.newsDirOverride != "" || tt.name == "news_system_fails_when_news_dir_unset" {
				origNews := news.GetNewsDir()
				news.SetNewsDir(tt.newsDirOverride)
				defer func() { news.SetNewsDir(origNews) }()
			}

			server := setupMockServer(tt.ghBody, tt.ghStatus)
			defer server.Close()

			originalTransport := http.DefaultTransport
			http.DefaultTransport = &mockTransport{serverURL: server.URL}
			defer func() { http.DefaultTransport = originalTransport }()

			originalPath := os.Getenv("PATH")
			os.Setenv("PATH", "")
			defer os.Setenv("PATH", originalPath)

			doc := NewDoctor()
			out := tt.execOutput
			if out == "" && tt.execExit == 0 {
				out = "go version go1.20 linux/amd64\n"
			}
			doc.Executor = &command.MockExecutor{
				MockOutput: out,
				ExitCode:   tt.execExit,
			}
			if tt.profileStore != nil {
				doc.ProfileStore = tt.profileStore
			}

			results, allHealthy := doc.RunAllChecks()

			if allHealthy != tt.wantStatus {
				t.Errorf("expected allHealthy %v, got %v", tt.wantStatus, allHealthy)
			}

			// Validate results map
			resMap := make(map[string]string)
			for _, r := range results {
				resMap[r.Name] = r.Status
			}

			for k, v := range tt.checkHas {
				if got, ok := resMap[k]; !ok || got != v {
					t.Errorf("expected check %q to be %q, got %q", k, v, got)
				}
			}
		})
	}
}

type mockTransport struct {
	serverURL string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only intercept github api
	if req.URL.Host == "api.github.com" {
		req.URL.Scheme = "http"
		req.URL.Host = m.serverURL[7:] // strip http://
	}
	// Fallback to a real transport if needed, but we shouldn't make real requests.
	// For testing, we can just use the default transport which will hit our local server.
	transport := http.DefaultTransport
	if transport == nil || transport == m {
		transport = new(http.Transport)
	}
	return transport.RoundTrip(req)
}
