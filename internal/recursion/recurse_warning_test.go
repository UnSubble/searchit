package recursion_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/status"
)

func captureStderr(fn func()) string {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	fn()

	_ = w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// TestRecurseWarning_Root404_DefaultPolicy verifies that root 404 with default recurse-on displays warning.
func TestRecurseWarning_Root404_DefaultPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // 404
	}))
	t.Cleanup(srv.Close)

	a := newApp(t)
	m := recursion.NewManager(
		a.HTTPClient,
		nil,
		staticReader{words: []string{"a"}},
		recursion.BFS,
		2,
		status.MustParse("200,301,302,403"), // default
		false,
		false,
		0,
		nil,
		nil,
		100,
	)

	stderr := captureStderr(func() {
		m.Run(context.Background(), context.Background(), []string{srv.URL + "/"}, 2, func(r engine.Result) {})
	})

	if !strings.Contains(stderr, "[!] The root URL returned HTTP 404.") {
		t.Errorf("expected root 404 warning in stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "--recurse-on: 200,301,302,403") {
		t.Errorf("expected policy string in stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "--recurse-on 200,301,302,403,404") {
		t.Errorf("expected suggestion in stderr, got:\n%s", stderr)
	}
}

// TestRecurseWarning_Root403_ExcludedPolicy verifies that root 403 when excluded displays warning.
func TestRecurseWarning_Root403_ExcludedPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden) // 403
	}))
	t.Cleanup(srv.Close)

	a := newApp(t)
	m := recursion.NewManager(
		a.HTTPClient,
		nil,
		staticReader{words: []string{"a"}},
		recursion.BFS,
		2,
		status.MustParse("200,301,302"), // excludes 403
		false,
		false,
		0,
		nil,
		nil,
		100,
	)

	stderr := captureStderr(func() {
		m.Run(context.Background(), context.Background(), []string{srv.URL + "/"}, 2, func(r engine.Result) {})
	})

	if !strings.Contains(stderr, "[!] The root URL returned HTTP 403.") {
		t.Errorf("expected root 403 warning in stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "--recurse-on: 200,301,302") {
		t.Errorf("expected policy string in stderr, got:\n%s", stderr)
	}
}

// TestRecurseWarning_Root500_StatusExpression verifies wildcards like 2xx,3xx displaying warning when root 500.
func TestRecurseWarning_Root500_StatusExpression(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // 500
	}))
	t.Cleanup(srv.Close)

	a := newApp(t)
	m := recursion.NewManager(
		a.HTTPClient,
		nil,
		staticReader{words: []string{"a"}},
		recursion.BFS,
		2,
		status.MustParse("2xx,3xx"), // wildcard status expression
		false,
		false,
		0,
		nil,
		nil,
		100,
	)

	stderr := captureStderr(func() {
		m.Run(context.Background(), context.Background(), []string{srv.URL + "/"}, 2, func(r engine.Result) {})
	})

	if !strings.Contains(stderr, "[!] The root URL returned HTTP 500.") {
		t.Errorf("expected root 500 warning in stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "--recurse-on: 2xx,3xx") {
		t.Errorf("expected policy string 2xx,3xx in stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "--recurse-on 2xx,3xx,500") {
		t.Errorf("expected suggestion in stderr, got:\n%s", stderr)
	}
}

// TestRecurseWarning_Root200_NoWarning verifies root 200 produces no warning.
func TestRecurseWarning_Root200_NoWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // 200
	}))
	t.Cleanup(srv.Close)

	a := newApp(t)
	m := recursion.NewManager(
		a.HTTPClient,
		nil,
		staticReader{words: []string{"a"}},
		recursion.BFS,
		2,
		status.MustParse("200,301,302,403"),
		false,
		false,
		0,
		nil,
		nil,
		100,
	)

	stderr := captureStderr(func() {
		m.Run(context.Background(), context.Background(), []string{srv.URL + "/"}, 2, func(r engine.Result) {})
	})

	if strings.Contains(stderr, "[!] The root URL returned") {
		t.Errorf("unexpected warning in stderr for root 200:\n%s", stderr)
	}
}

// TestRecurseWarning_TransportError_NoWarning verifies transport error produces no warning.
func TestRecurseWarning_TransportError_NoWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // Closed server -> transport error

	a := newApp(t)
	m := recursion.NewManager(
		a.HTTPClient,
		nil,
		staticReader{words: []string{"a"}},
		recursion.BFS,
		2,
		status.MustParse("200,301,302,403"),
		false,
		false,
		0,
		nil,
		nil,
		100,
	)

	stderr := captureStderr(func() {
		m.Run(context.Background(), context.Background(), []string{srv.URL + "/"}, 2, func(r engine.Result) {})
	})

	if strings.Contains(stderr, "[!] The root URL returned") {
		t.Errorf("unexpected recurse-on warning in stderr for transport error:\n%s", stderr)
	}
}
