package filter

import (
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
