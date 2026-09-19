package fuzz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
)

func TestRunner_BuildJob_Encoding(t *testing.T) {
	b64Enc, _ := NewEncoder("base64")
	urlEnc, _ := NewEncoder("url")
	doubleURLEnc, _ := NewEncoder("doubleurl")

	tests := []struct {
		name        string
		encoder     Encoder
		targetURL   string
		vars        map[string]string
		wantURL     string
		bodyTmpl    string
		wantBody    string
		headerTmpl  http.Header
		wantHeaders map[string]string
		cookieTmpl  string
		wantCookies []string
	}{
		{
			name:      "no encoding - default unchanged",
			encoder:   nil,
			targetURL: "http://example.com/api/FUZZ/test",
			vars:      map[string]string{"FUZZ": "admin/user"},
			wantURL:   "http://example.com/api/admin/user/test",
		},
		{
			name:      "url encoding - embedded URL placeholder with static parts preserved",
			encoder:   urlEnc,
			targetURL: "http://example.com/api/FUZZ/test",
			vars:      map[string]string{"FUZZ": "admin/user"},
			wantURL:   "http://example.com/api/admin%2Fuser/test",
		},
		{
			name:      "url encoding with extension variant",
			encoder:   urlEnc,
			targetURL: "http://example.com/api/FUZZ/test",
			vars:      map[string]string{"FUZZ": "admin/user.php"},
			wantURL:   "http://example.com/api/admin%2Fuser.php/test",
		},
		{
			name:      "doubleurl encoding - embedded URL placeholder",
			encoder:   doubleURLEnc,
			targetURL: "http://example.com/api/FUZZ/test",
			vars:      map[string]string{"FUZZ": "admin/user"},
			wantURL:   "http://example.com/api/admin%252Fuser/test",
		},
		{
			name:      "base64 encoding - standard base64",
			encoder:   b64Enc,
			targetURL: "http://example.com/FUZZ",
			vars:      map[string]string{"FUZZ": "admin/user"},
			wantURL:   "http://example.com/YWRtaW4vdXNlcg==",
		},
		{
			name:      "multiple placeholders - FUZZ and FOO both encoded",
			encoder:   urlEnc,
			targetURL: "http://example.com/FUZZ?filter=FOO",
			vars:      map[string]string{"FUZZ": "admin/user", "FOO": "test space&special"},
			wantURL:   "http://example.com/admin%2Fuser?filter=test%20space%26special",
		},
		{
			name:       "header placeholder encoding",
			encoder:    b64Enc,
			targetURL:  "http://example.com/index",
			headerTmpl: http.Header{"X-Auth": []string{"Bearer FUZZ"}},
			vars:       map[string]string{"FUZZ": "admin:pass"},
			wantURL:    "http://example.com/index",
			wantHeaders: map[string]string{
				"X-Auth": "Bearer YWRtaW46cGFzcw==",
			},
		},
		{
			name:      "body placeholder encoding",
			encoder:   doubleURLEnc,
			targetURL: "http://example.com/login",
			bodyTmpl:  "username=FUZZ",
			vars:      map[string]string{"FUZZ": "admin/user"},
			wantURL:   "http://example.com/login",
			wantBody:  "username=admin%252Fuser",
		},
		{
			name:        "cookie placeholder encoding",
			encoder:     urlEnc,
			targetURL:   "http://example.com/profile",
			cookieTmpl:  "session=FUZZ",
			vars:        map[string]string{"FUZZ": "user/token="},
			wantURL:     "http://example.com/profile",
			wantCookies: []string{"session=user%2Ftoken%3D"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Runner{
				TargetURL:       tt.targetURL,
				Method:          "GET",
				BodyTemplate:    tt.bodyTmpl,
				HeaderTemplates: tt.headerTmpl,
				CookieTemplate:  tt.cookieTmpl,
				Encoder:         tt.encoder,
			}
			r.PrepareTemplates()

			job, err := r.BuildJob(r.CompiledURLTemplate(), tt.vars)
			if err != nil {
				t.Fatalf("BuildJob error: %v", err)
			}

			if job.URL != tt.wantURL {
				t.Errorf("URL mismatch:\n got:  %s\n want: %s", job.URL, tt.wantURL)
			}

			if tt.wantBody != "" && job.Body != tt.wantBody {
				t.Errorf("Body mismatch:\n got:  %s\n want: %s", job.Body, tt.wantBody)
			}

			for hKey, hWant := range tt.wantHeaders {
				vals := job.Headers[hKey]
				if len(vals) == 0 || vals[0] != hWant {
					t.Errorf("Header %q mismatch:\n got:  %v\n want: %s", hKey, vals, hWant)
				}
			}

			if len(tt.wantCookies) > 0 {
				if len(job.Cookies) != len(tt.wantCookies) || job.Cookies[0] != tt.wantCookies[0] {
					t.Errorf("Cookies mismatch:\n got:  %v\n want: %v", job.Cookies, tt.wantCookies)
				}
			}
		})
	}
}

func TestRunner_DryRun_Parity_WithEncoder(t *testing.T) {
	for _, encName := range []string{"base64", "url", "doubleurl"} {
		t.Run(encName, func(t *testing.T) {
			enc, err := NewEncoder(encName)
			if err != nil {
				t.Fatalf("NewEncoder(%s) failed: %v", encName, err)
			}

			words := []string{"admin/user", "test.php", "special/dir"}

			var receivedURLs []string
			var mu sync.Mutex
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				receivedURLs = append(receivedURLs, r.URL.RequestURI())
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			runner := &Runner{
				TargetURL: srv.URL + "/api/FUZZ/v1",
				Method:    "GET",
				Encoder:   enc,
			}
			runner.PrepareTemplates()

			// 1. Generate via dry-run
			pChan1 := make(chan string, len(words))
			for _, w := range words {
				pChan1 <- w
			}
			close(pChan1)

			dryRequests, total, err := GenerateDryRunRequests(context.Background(), runner, pChan1, 10)
			if err != nil {
				t.Fatalf("GenerateDryRunRequests failed: %v", err)
			}
			if total != int64(len(words)) {
				t.Fatalf("expected total %d, got %d", len(words), total)
			}

			// 2. Verify each dry-run URL matches expected encoded format
			for i, dr := range dryRequests {
				expectedEncoded := enc.Encode(words[i])
				expectedURL := srv.URL + "/api/" + expectedEncoded + "/v1"
				if dr.Req.URL != expectedURL {
					t.Errorf("dry-run request %d URL mismatch: got %q, want %q", i+1, dr.Req.URL, expectedURL)
				}
			}
		})
	}
}

func TestRunner_FuzzData_HasEncodedValues(t *testing.T) {
	enc, _ := NewEncoder("url")
	r := &Runner{
		TargetURL:       "http://example.com/FUZZ",
		Method:          "POST",
		BodyTemplate:    "param=FOO",
		HeaderTemplates: http.Header{"X-Header": []string{"BAR"}},
		CookieTemplate:  "auth=BAZ",
		Encoder:         enc,
	}
	r.PrepareTemplates()

	vars := map[string]string{
		"FUZZ": "dir/target",
		"FOO":  "body/val&1",
		"BAR":  "header/val",
		"BAZ":  "cookie/val",
	}

	job, err := r.BuildJob(r.CompiledURLTemplate(), vars)
	if err != nil {
		t.Fatalf("BuildJob failed: %v", err)
	}

	if job.FuzzData == nil {
		t.Fatalf("expected non-nil FuzzData")
	}

	foundHeader := false
	foundBody := false
	foundCookie := false

	for _, f := range job.FuzzData.Fields {
		switch f.Location {
		case engine.LocationHeader:
			if f.Value == "header%2Fval" {
				foundHeader = true
			}
		case engine.LocationBody:
			if strings.Contains(f.Value, "body%2Fval%261") {
				foundBody = true
			}
		case engine.LocationCookie:
			if f.Value == "cookie%2Fval" {
				foundCookie = true
			}
		}
	}

	if !foundHeader {
		t.Errorf("expected encoded header in FuzzData")
	}
	if !foundBody {
		t.Errorf("expected encoded body in FuzzData")
	}
	if !foundCookie {
		t.Errorf("expected encoded cookie in FuzzData")
	}
}
