package github

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupMockServer(releasesBody string, releasesStatus int, tagsBody string, tagsStatus int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/UnSubble/searchit/releases" {
			w.WriteHeader(releasesStatus)
			fmt.Fprint(w, releasesBody)
			return
		}
		if r.URL.Path == "/repos/UnSubble/searchit/tags" {
			w.WriteHeader(tagsStatus)
			fmt.Fprint(w, tagsBody)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestFetchVersions(t *testing.T) {
	releasesJSON := `[
		{"tag_name": "v1.0.0", "draft": false},
		{"tag_name": "v1.1.0-beta", "draft": false},
		{"tag_name": "v1.2.0", "draft": true}
	]`
	tagsJSON := `[
		{"name": "v0.9.0"},
		{"name": "v0.9.1"}
	]`

	tests := []struct {
		name           string
		releasesBody   string
		releasesStatus int
		tagsBody       string
		tagsStatus     int
		wantErr        bool
		expected       []string
	}{
		{
			name:           "releases_pass",
			releasesBody:   releasesJSON,
			releasesStatus: 200,
			tagsBody:       tagsJSON,
			tagsStatus:     200, // Should be ignored since releases pass
			wantErr:        false,
			expected:       []string{"v1.1.0-beta", "v1.0.0"}, // sorted, drafts ignored
		},
		{
			name:           "releases_fail_tags_pass",
			releasesBody:   `{}`,
			releasesStatus: 500,
			tagsBody:       tagsJSON,
			tagsStatus:     200,
			wantErr:        false,
			expected:       []string{"v0.9.1", "v0.9.0"}, // sorted
		},
		{
			name:           "all_fail",
			releasesBody:   `{}`,
			releasesStatus: 500,
			tagsBody:       `{}`,
			tagsStatus:     500,
			wantErr:        true,
		},
		{
			name:           "malformed_json_fallback",
			releasesBody:   `[invalid json`,
			releasesStatus: 200,
			tagsBody:       tagsJSON,
			tagsStatus:     200,
			wantErr:        false,
			expected:       []string{"v0.9.1", "v0.9.0"},
		},
		{
			name:           "rate_limit_fallback",
			releasesBody:   `{"message": "API rate limit exceeded"}`,
			releasesStatus: 403,
			tagsBody:       tagsJSON,
			tagsStatus:     200,
			wantErr:        false,
			expected:       []string{"v0.9.1", "v0.9.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupMockServer(tt.releasesBody, tt.releasesStatus, tt.tagsBody, tt.tagsStatus)
			defer server.Close()

			// Use the httptest client to automatically override the URL globally is tricky
			// We should patch apiBase
			oldBase := apiBase
			apiBase = server.URL
			defer func() { apiBase = oldBase }()

			client := NewClient()

			versions, err := client.FetchVersions()
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchVersions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(versions) != len(tt.expected) {
					t.Fatalf("expected %d versions, got %d", len(tt.expected), len(versions))
				}
				for i, v := range versions {
					if v.Original != tt.expected[i] {
						t.Errorf("expected version %d to be %s, got %s", i, tt.expected[i], v.Original)
					}
				}
			}
		})
	}
}

func TestGetLatestStableAndLatest(t *testing.T) {
	releasesJSON := `[
		{"tag_name": "v1.0.0", "draft": false},
		{"tag_name": "v1.1.0-beta", "draft": false}
	]`

	server := setupMockServer(releasesJSON, 200, "", 200)
	defer server.Close()

	oldBase := apiBase
	apiBase = server.URL
	defer func() { apiBase = oldBase }()

	client := NewClient()

	stable, err := client.GetLatestStable()
	if err != nil {
		t.Fatal(err)
	}
	if stable.Original != "v1.0.0" {
		t.Errorf("expected stable v1.0.0, got %s", stable.Original)
	}

	latest, err := client.GetLatest()
	if err != nil {
		t.Fatal(err)
	}
	if latest.Original != "v1.1.0-beta" {
		t.Errorf("expected latest v1.1.0-beta, got %s", latest.Original)
	}
}
