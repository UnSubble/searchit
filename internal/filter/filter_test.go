package filter

import (
	"net/http"
	"testing"
)

func TestFilterSuite_StatusAndSize(t *testing.T) {
	fs, err := NewFilterSuite("200,3xx", "301", "100-200", "150", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	// Test Status matching: 200 matches mc (200), not fc
	if !fs.MatchHeaders(200, 120, "text/html") {
		t.Error("expected 200 to match headers")
	}

	// Test Status match wildcard: 302 matches mc (3xx), not fc
	if !fs.MatchHeaders(302, 120, "text/html") {
		t.Error("expected 302 to match headers")
	}

	// Test Status filter: 301 is filtered by fc (301)
	if fs.MatchHeaders(301, 120, "text/html") {
		t.Error("expected 301 to be filtered")
	}

	// Test Status mismatch: 404 does not match mc
	if fs.MatchHeaders(404, 120, "text/html") {
		t.Error("expected 404 to be filtered (not in mc)")
	}

	// Test Size matching: 120 matches ms (100-200) and not fs (150)
	if !fs.MatchHeaders(200, 120, "text/html") {
		t.Error("expected size 120 to match headers")
	}

	// Test Size filter: 150 matches fs (150)
	if fs.MatchHeaders(200, 150, "text/html") {
		t.Error("expected size 150 to be filtered")
	}

	// Test Size mismatch: 250 does not match ms (100-200)
	if fs.MatchHeaders(200, 250, "text/html") {
		t.Error("expected size 250 to be filtered")
	}
}

func TestFilterSuite_ContentType(t *testing.T) {
	fs, err := NewFilterSuite("", "", "", "", nil, nil, []string{"application/json", "text/html"}, []string{"text/plain"})
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	// Test match content type
	if !fs.MatchHeaders(200, 100, "application/json; charset=utf-8") {
		t.Error("expected application/json to match")
	}
	if !fs.MatchHeaders(200, 100, "TEXT/HTML") {
		t.Error("expected TEXT/HTML to match (case insensitive)")
	}

	// Test filter content type
	if fs.MatchHeaders(200, 100, "text/plain") {
		t.Error("expected text/plain to be filtered")
	}

	// Test mismatch
	if fs.MatchHeaders(200, 100, "image/png") {
		t.Error("expected image/png to be filtered (not in mt)")
	}
}

func TestFilterSuite_Regex(t *testing.T) {
	fs, err := NewFilterSuite("", "", "", "", []string{"admin", "token-[0-9]+"}, []string{"invalid"}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	if !fs.RequiresBody() {
		t.Error("expected RequiresBody to be true")
	}

	// Test match regex
	if !fs.MatchBody([]byte("hello admin page")) {
		t.Error("expected 'admin' regex to match")
	}
	if !fs.MatchBody([]byte("token-12345 key")) {
		t.Error("expected 'token-[0-9]+' regex to match")
	}

	// Test filter regex
	if fs.MatchBody([]byte("hello admin invalid token")) {
		t.Error("expected 'invalid' filter regex to filter out")
	}

	// Test mismatch regex
	if fs.MatchBody([]byte("regular page content")) {
		t.Error("expected mismatch to filter out")
	}
}

func TestFilterSuite_CompilationError(t *testing.T) {
	_, err := NewFilterSuite("", "", "", "", []string{"["}, nil, nil, nil)
	if err == nil {
		t.Error("expected compile error for invalid match-regex")
	}

	_, err = NewFilterSuite("", "", "", "", nil, []string{"*"}, nil, nil)
	if err == nil {
		t.Error("expected compile error for invalid filter-regex")
	}
}

func TestFilterSuite_RegexScopes(t *testing.T) {
	headers := http.Header{
		"Server":       []string{"nginx/1.24.0"},
		"Content-Type": []string{"text/html; charset=utf-8"},
		"X-Custom":     []string{"secret-header-value"},
	}

	tests := []struct {
		name      string
		scope     string
		mr        []string
		fr        []string
		url       string
		status    int
		headers   http.Header
		body      []byte
		wantMatch bool
	}{
		// URL scope
		{
			name:      "scope url: match target in URL",
			scope:     RegexScopeURL,
			mr:        []string{`api/v[0-9]+`},
			url:       "http://example.com/api/v2/users",
			status:    200,
			headers:   headers,
			body:      []byte("hello world"),
			wantMatch: true,
		},
		{
			name:      "scope url: pattern not in URL",
			scope:     RegexScopeURL,
			mr:        []string{`api/v[0-9]+`},
			url:       "http://example.com/auth/login",
			status:    200,
			headers:   headers,
			body:      []byte("api/v2 in body"),
			wantMatch: false,
		},
		{
			name:      "scope url: filter regex rejects matching URL",
			scope:     RegexScopeURL,
			fr:        []string{`logout`},
			url:       "http://example.com/auth/logout",
			status:    200,
			headers:   headers,
			body:      []byte("safe content"),
			wantMatch: false,
		},

		// Header scope
		{
			name:      "scope header: match server header",
			scope:     RegexScopeHeader,
			mr:        []string{`(?i)server:\s*nginx`},
			url:       "http://example.com/",
			status:    200,
			headers:   headers,
			body:      []byte("some body"),
			wantMatch: true,
		},
		{
			name:      "scope header: match custom header",
			scope:     RegexScopeHeader,
			mr:        []string{`X-Custom:\s*secret`},
			url:       "http://example.com/",
			status:    200,
			headers:   headers,
			body:      []byte("some body"),
			wantMatch: true,
		},
		{
			name:      "scope header: filter regex rejects matching header",
			scope:     RegexScopeHeader,
			fr:        []string{`(?i)nginx`},
			url:       "http://example.com/",
			status:    200,
			headers:   headers,
			body:      []byte("some body"),
			wantMatch: false,
		},

		// Status scope
		{
			name:      "scope status: match 200",
			scope:     RegexScopeStatus,
			mr:        []string{`^20[0-4]$`},
			url:       "http://example.com/",
			status:    200,
			headers:   headers,
			body:      []byte("some body"),
			wantMatch: true,
		},
		{
			name:      "scope status: mismatch status",
			scope:     RegexScopeStatus,
			mr:        []string{`^40[0-4]$`},
			url:       "http://example.com/",
			status:    200,
			headers:   headers,
			body:      []byte("404 in body"),
			wantMatch: false,
		},
		{
			name:      "scope status: filter regex excludes 500",
			scope:     RegexScopeStatus,
			fr:        []string{`^5\d\d$`},
			url:       "http://example.com/",
			status:    500,
			headers:   headers,
			body:      []byte("some body"),
			wantMatch: false,
		},

		// Body scope
		{
			name:      "scope body: match text in body",
			scope:     RegexScopeBody,
			mr:        []string{`admin_panel`},
			url:       "http://example.com/",
			status:    200,
			headers:   headers,
			body:      []byte("welcome to admin_panel v1"),
			wantMatch: true,
		},
		{
			name:      "scope body: pattern only in URL is not matched",
			scope:     RegexScopeBody,
			mr:        []string{`admin_panel`},
			url:       "http://example.com/admin_panel",
			status:    200,
			headers:   headers,
			body:      []byte("plain welcome page"),
			wantMatch: false,
		},

		// All scope
		{
			name:      "scope all: matches if found in URL",
			scope:     RegexScopeAll,
			mr:        []string{`unique-url-token`},
			url:       "http://example.com/unique-url-token",
			status:    200,
			headers:   headers,
			body:      []byte("content"),
			wantMatch: true,
		},
		{
			name:      "scope all: matches if found in status",
			scope:     RegexScopeAll,
			mr:        []string{`^404$`},
			url:       "http://example.com/",
			status:    404,
			headers:   headers,
			body:      []byte("content"),
			wantMatch: true,
		},
		{
			name:      "scope all: matches if found in header",
			scope:     RegexScopeAll,
			mr:        []string{`secret-header-value`},
			url:       "http://example.com/",
			status:    200,
			headers:   headers,
			body:      []byte("content"),
			wantMatch: true,
		},
		{
			name:      "scope all: matches if found in body",
			scope:     RegexScopeAll,
			mr:        []string{`secret-body-value`},
			url:       "http://example.com/",
			status:    200,
			headers:   headers,
			body:      []byte("this is secret-body-value here"),
			wantMatch: true,
		},
		{
			name:      "scope all: excluded if filter regex matches any field",
			scope:     RegexScopeAll,
			fr:        []string{`secret-header-value`},
			url:       "http://example.com/",
			status:    200,
			headers:   headers,
			body:      []byte("innocent body"),
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs, err := NewFilterSuite("", "", "", "", tt.mr, tt.fr, nil, nil, tt.scope)
			if err != nil {
				t.Fatalf("unexpected error creating FilterSuite: %v", err)
			}
			matched := fs.MatchResponse(tt.url, tt.status, tt.headers, tt.body)
			if matched != tt.wantMatch {
				t.Errorf("MatchResponse() = %v, want %v", matched, tt.wantMatch)
			}
		})
	}
}

func TestFilterSuite_RequiresBodyOptimization(t *testing.T) {
	// url scope with regex does NOT require body
	fsURL, err := NewFilterSuite("", "", "", "", []string{"test"}, nil, nil, nil, RegexScopeURL)
	if err != nil {
		t.Fatal(err)
	}
	if fsURL.RequiresBody() {
		t.Errorf("expected RequiresBody() == false for RegexScopeURL")
	}

	// header scope with regex does NOT require body
	fsHdr, err := NewFilterSuite("", "", "", "", []string{"test"}, nil, nil, nil, RegexScopeHeader)
	if err != nil {
		t.Fatal(err)
	}
	if fsHdr.RequiresBody() {
		t.Errorf("expected RequiresBody() == false for RegexScopeHeader")
	}

	// status scope with regex does NOT require body
	fsStatus, err := NewFilterSuite("", "", "", "", []string{"test"}, nil, nil, nil, RegexScopeStatus)
	if err != nil {
		t.Fatal(err)
	}
	if fsStatus.RequiresBody() {
		t.Errorf("expected RequiresBody() == false for RegexScopeStatus")
	}

	// body scope with regex DOES require body
	fsBody, err := NewFilterSuite("", "", "", "", []string{"test"}, nil, nil, nil, RegexScopeBody)
	if err != nil {
		t.Fatal(err)
	}
	if !fsBody.RequiresBody() {
		t.Errorf("expected RequiresBody() == true for RegexScopeBody")
	}

	// all scope with regex DOES require body
	fsAll, err := NewFilterSuite("", "", "", "", []string{"test"}, nil, nil, nil, RegexScopeAll)
	if err != nil {
		t.Fatal(err)
	}
	if !fsAll.RequiresBody() {
		t.Errorf("expected RequiresBody() == true for RegexScopeAll")
	}
}

func TestFilterSuite_InvalidScope(t *testing.T) {
	_, err := NewFilterSuite("", "", "", "", []string{"test"}, nil, nil, nil, "invalid-scope")
	if err == nil {
		t.Error("expected error for invalid scope, got nil")
	}
}
