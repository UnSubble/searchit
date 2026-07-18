package robots_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/robots"
)

type mockTransport struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

func TestDownload_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/robots.txt" {
			t.Errorf("expected path /robots.txt, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("User-agent: *\nDisallow: /admin"))
	}))
	defer srv.Close()

	body, robotsURL, err := robots.Download(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer body.Close()

	if !strings.HasSuffix(robotsURL, "/robots.txt") {
		t.Errorf("expected robotsURL to end with /robots.txt, got %s", robotsURL)
	}

	content, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if !strings.Contains(string(content), "Disallow: /admin") {
		t.Errorf("unexpected content: %s", string(content))
	}
}

func TestDownload_InvalidTargetURL(t *testing.T) {
	_, _, err := robots.Download(context.Background(), http.DefaultClient, "http://[invalid-ipv6-address")
	if err == nil {
		t.Fatal("expected error parsing invalid target URL, got nil")
	}
}

func TestDownload_ClientError(t *testing.T) {
	client := &http.Client{
		Transport: &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				return nil, io.ErrUnexpectedEOF
			},
		},
	}

	_, _, err := robots.Download(context.Background(), client, "http://localhost")
	if err == nil || !strings.Contains(err.Error(), io.ErrUnexpectedEOF.Error()) {
		t.Fatalf("expected error containing %q, got: %v", io.ErrUnexpectedEOF, err)
	}
}

func TestDownload_UnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, _, err := robots.Download(context.Background(), srv.Client(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "unexpected status code: 404") {
		t.Fatalf("expected unexpected status code 404 error, got: %v", err)
	}
}

func TestDownload_CancelledContext(t *testing.T) {
	client := &http.Client{
		Transport: &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				// Wait for context cancellation
				<-req.Context().Done()
				return nil, req.Context().Err()
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, _, err := robots.Download(ctx, client, "http://localhost")
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("expected error containing %q, got: %v", context.Canceled, err)
	}
}
