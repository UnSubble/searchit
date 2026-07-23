package update

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/unsubble/searchit/internal/news"
	"github.com/unsubble/searchit/internal/semver"
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

func TestCheck(t *testing.T) {
	releasesJSON := `[
		{"tag_name": "v1.0.0", "draft": false},
		{"tag_name": "v1.1.0-beta", "draft": false},
		{"tag_name": "v0.9.0", "draft": false}
	]`

	server := setupMockServer(releasesJSON, 200)
	defer server.Close()

	tests := []struct {
		name             string
		currentVer       string
		targetVersionStr string
		experimental     bool
		isRollback       bool
		wantStatus       string
		wantIsUpdate     bool
		wantIsDowngrade  bool
		wantIsRollback   bool
		wantErr          bool
	}{
		{
			name:             "upgrade_to_stable",
			currentVer:       "v0.9.0",
			targetVersionStr: "",
			experimental:     false,
			isRollback:       false,
			wantStatus:       "UPDATE AVAILABLE",
			wantIsUpdate:     true,
		},
		{
			name:             "upgrade_to_experimental",
			currentVer:       "v0.9.0",
			targetVersionStr: "",
			experimental:     true,
			isRollback:       false,
			wantStatus:       "UPDATE AVAILABLE",
			wantIsUpdate:     true,
		},
		{
			name:             "already_up_to_date",
			currentVer:       "v1.0.0",
			targetVersionStr: "",
			experimental:     false,
			isRollback:       false,
			wantStatus:       "UP TO DATE",
		},
		{
			name:             "explicit_downgrade",
			currentVer:       "v1.0.0",
			targetVersionStr: "v0.9.0",
			experimental:     false,
			isRollback:       false,
			wantStatus:       "DOWNGRADE REQUESTED (WARNING)",
			wantIsDowngrade:  true,
		},
		{
			name:             "explicit_rollback",
			currentVer:       "v1.0.0",
			targetVersionStr: "v0.9.0",
			experimental:     false,
			isRollback:       true,
			wantStatus:       "ROLLBACK AVAILABLE",
			wantIsDowngrade:  true,
			wantIsRollback:   true,
		},
		{
			name:             "rollback_to_newer_version_warning",
			currentVer:       "v0.9.0",
			targetVersionStr: "v1.0.0",
			experimental:     false,
			isRollback:       true,
			wantStatus:       "ROLLBACK TARGET IS NEWER (POSSIBLY INCORRECT)",
			wantIsUpdate:     true,
		},
		{
			name:             "invalid_target",
			currentVer:       "v1.0.0",
			targetVersionStr: "invalid",
			experimental:     false,
			isRollback:       false,
			wantStatus:       "",
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldVersion := version.Version
			version.Version = tt.currentVer
			defer func() { version.Version = oldVersion }()

			mgr := NewManager()
			oldClient := mgr.Client

			// Monkey patch a custom RoundTripper
			mgr.Client.HTTPClient.Transport = &mockTransport{serverURL: server.URL}

			res, err := mgr.Check(tt.experimental, tt.targetVersionStr, tt.isRollback)

			if (err != nil) != tt.wantErr {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
			if tt.wantErr {
				return
			}

			if res.Status != tt.wantStatus {
				t.Errorf("expected status %q, got %q", tt.wantStatus, res.Status)
			}
			if res.IsUpdate != tt.wantIsUpdate {
				t.Errorf("expected IsUpdate %v, got %v", tt.wantIsUpdate, res.IsUpdate)
			}
			if res.IsDowngrade != tt.wantIsDowngrade {
				t.Errorf("expected IsDowngrade %v, got %v", tt.wantIsDowngrade, res.IsDowngrade)
			}
			if res.IsRollback != tt.wantIsRollback {
				t.Errorf("expected IsRollback %v, got %v", tt.wantIsRollback, res.IsRollback)
			}

			// Restore
			mgr.Client = oldClient
		})
	}
}

type mockTransport struct {
	serverURL string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite URL to mock server
	req.URL.Scheme = "http"
	req.URL.Host = m.serverURL[7:] // strip http://
	return http.DefaultTransport.RoundTrip(req)
}

func TestPreviewAction(t *testing.T) {
	tempDir := t.TempDir()
	newsDir := filepath.Join(tempDir, "var")
	os.Mkdir(newsDir, 0755)
	os.WriteFile(filepath.Join(newsDir, "v1.0.0.md"), []byte("News content"), 0644)

	oldNews := news.GetNewsDir()
	news.SetNewsDir(newsDir)
	defer news.SetNewsDir(oldNews)

	mgr := NewManager()
	target, _ := semver.Parse("v1.0.0")
	res := CheckResult{
		IsValidTarget: true,
		TargetVersion: target,
		Status:        "UPDATE AVAILABLE",
		IsUpdate:      true,
	}
	mgr.PreviewAction(res, "update")

	res.IsDowngrade = true
	mgr.PreviewAction(res, "update")

	mgr.PreviewAction(res, "rollback")
}

func TestExecute(t *testing.T) {
	mgr := &Manager{
		Executor: &command.MockExecutor{
			MockOutput: "success",
			ExitCode:   0,
		},
	}

	// Test success
	target, _ := semver.Parse("v1.0.0")
	err := mgr.Execute(target)
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
}
