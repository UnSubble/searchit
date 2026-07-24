package doctor

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/unsubble/searchit/internal/testutil/command"
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

func TestRunAllChecks(t *testing.T) {
	tests := []struct {
		name       string
		ghBody     string
		ghStatus   int
		wantStatus bool // true = HEALTHY, false = NOT READY
		checkHas   map[string]string
	}{
		{
			name:       "healthy_system",
			ghBody:     `[{"tag_name": "v1.0.0", "draft": false}]`,
			ghStatus:   200,
			wantStatus: false, // INSTALLATION METHOD will be WARNING
			checkHas: map[string]string{
				"UPDATE SYSTEM":       "PASS",
				"GITHUB CONNECTIVITY": "PASS",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupMockServer(tt.ghBody, tt.ghStatus)
			defer server.Close()

			originalTransport := http.DefaultTransport
			http.DefaultTransport = &mockTransport{serverURL: server.URL}
			defer func() { http.DefaultTransport = originalTransport }()

			originalPath := os.Getenv("PATH")
			os.Setenv("PATH", "")
			defer os.Setenv("PATH", originalPath)

			doc := NewDoctor()
			doc.Executor = &command.MockExecutor{
				MockOutput: "go version go1.20 linux/amd64\n",
				ExitCode:   0,
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
