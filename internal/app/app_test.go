package app_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/fingerprint"
)

func TestNew_UsesBackgroundContextWhenNil(t *testing.T) {
	var nilCtx context.Context
	a := app.New(nilCtx, config.Default())
	if a.Context == nil {
		t.Fatal("Context is nil, want context.Background()")
	}
}

func TestNew_PreservesProvidedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := app.New(ctx, config.Default())
	if a.Context != ctx {
		t.Error("Context was replaced; want the provided context")
	}
}

func TestNew_CreatesHTTPClient(t *testing.T) {
	a := app.New(context.Background(), config.Default())
	if a.HTTPClient == nil {
		t.Fatal("HTTPClient is nil")
	}
}

func TestNew_ConfiguresHTTPClientTimeout(t *testing.T) {
	cfg := config.Default()
	cfg.Timeout = 30 * time.Second

	a := app.New(context.Background(), cfg)

	if want := 30 * time.Second; a.HTTPClient.Timeout != want {
		t.Errorf("HTTPClient.Timeout = %v, want %v", a.HTTPClient.Timeout, want)
	}
}

type mockRoundTripper struct {
	resp *http.Response
	err  error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.resp, m.err
}

func TestWrapTransport_NilUnderlying(t *testing.T) {
	rt := app.WrapTransport(nil, nil)
	if rt == nil {
		t.Fatal("WrapTransport returned nil")
	}
}

func TestFingerprintRoundTripper_RoundTripError(t *testing.T) {
	mockErr := testing.TB(t).Name() + " mock error"
	rt := app.WrapTransport(&mockRoundTripper{err: fmt.Errorf("%s", mockErr)}, nil)

	req, _ := http.NewRequest(http.MethodGet, "http://localhost", nil)
	_, err := rt.RoundTrip(req)
	if err == nil || !strings.Contains(err.Error(), mockErr) {
		t.Fatalf("expected error containing %q, got: %v", mockErr, err)
	}
}

func TestFingerprintRoundTripper_NilResponse(t *testing.T) {
	rt := app.WrapTransport(&mockRoundTripper{resp: nil}, nil)

	req, _ := http.NewRequest(http.MethodGet, "http://localhost", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %v", resp)
	}
}

func TestFingerprintRoundTripper_NormalResponse(t *testing.T) {
	body := []byte("hello world")
	mockResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"X-Test": []string{"yes"}},
	}
	rt := app.WrapTransport(&mockRoundTripper{resp: mockResp}, nil)

	req, _ := http.NewRequest(http.MethodGet, "http://localhost/path", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Read restored body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if string(respBody) != "hello world" {
		t.Errorf("expected body %q, got %q", "hello world", string(respBody))
	}
}

func TestFingerprintRoundTripper_WithCache(t *testing.T) {
	cache := fingerprint.NewCache()
	body := []byte("PHP/8.1 Laravel")
	mockResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"X-Powered-By": []string{"PHP/8.1"}},
	}
	rt := app.WrapTransport(&mockRoundTripper{resp: mockResp}, cache)

	req, _ := http.NewRequest(http.MethodGet, "http://localhost/path", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	fp := cache.Get("localhost")
	if fp == nil {
		t.Fatal("expected fingerprint to be recorded in cache")
	}
	signals := fp.Signals()
	if len(signals) == 0 {
		t.Fatal("expected signals to be recorded in fingerprint")
	}
}

func TestNew_AdaptiveEnablesWrapTransport(t *testing.T) {
	cfg := config.Default()
	cfg.Adaptive = true

	a := app.New(context.Background(), cfg)
	if a.FingerprintCache == nil {
		t.Error("expected FingerprintCache to be initialized when adaptive is true")
	}
	if a.HTTPClient.Transport == nil {
		t.Error("expected HTTPClient.Transport to be wrapped, got nil")
	}
}
