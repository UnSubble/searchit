package targets_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/unsubble/searchit/internal/targets"
)

func TestHasExplicitScheme(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"http://example.com", true},
		{"https://example.com", true},
		{"HTTP://EXAMPLE.COM", true},
		{"HTTPS://EXAMPLE.COM", true},
		{"ftp://example.com", true},
		{"example.com", false},
		{"example.com:8080", false},
		{"localhost", false},
		{"127.0.0.1:8080", false},
		{"[::1]:8080", false},
		{"sub.domain.co.uk/path", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := targets.HasExplicitScheme(tt.url)
			if got != tt.want {
				t.Errorf("HasExplicitScheme(%q) = %v; want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestShouldFallbackToHTTP(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantFall bool
	}{
		{
			name:     "nil error",
			err:      nil,
			wantFall: false,
		},
		{
			name:     "connection refused net.OpError",
			err:      &net.OpError{Op: "dial", Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}},
			wantFall: true,
		},
		{
			name:     "connection refused string",
			err:      errors.New("dial tcp 127.0.0.1:443: connect: connection refused"),
			wantFall: true,
		},
		{
			name:     "EOF during handshake",
			err:      io.EOF,
			wantFall: true,
		},
		{
			name:     "unexpected EOF during handshake",
			err:      io.ErrUnexpectedEOF,
			wantFall: true,
		},
		{
			name:     "first record does not look like TLS",
			err:      errors.New("tls: first record does not look like a TLS handshake"),
			wantFall: true,
		},
		{
			name:     "server gave HTTP response to HTTPS client",
			err:      errors.New("net/http: HTTP/1.x transport connection broken: server gave HTTP response to HTTPS client"),
			wantFall: true,
		},
		{
			name:     "unknown authority (invalid cert)",
			err:      x509.UnknownAuthorityError{},
			wantFall: false,
		},
		{
			name:     "hostname mismatch",
			err:      x509.HostnameError{Host: "example.com", Certificate: &x509.Certificate{}},
			wantFall: false,
		},
		{
			name:     "expired certificate",
			err:      x509.CertificateInvalidError{Reason: x509.Expired},
			wantFall: false,
		},
		{
			name:     "generic x509 error string",
			err:      errors.New("Get \"https://example.com\": x509: certificate signed by unknown authority"),
			wantFall: false,
		},
		{
			name:     "user context canceled",
			err:      context.Canceled,
			wantFall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := targets.ShouldFallbackToHTTP(tt.err)
			if got != tt.wantFall {
				t.Errorf("ShouldFallbackToHTTP() = %v; want %v for err: %v", got, tt.wantFall, tt.err)
			}
		})
	}
}

func TestAutoDetectTarget(t *testing.T) {
	ctx := context.Background()

	// 1. Plain HTTP Server (No TLS)
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer httpSrv.Close()

	// 2. Valid HTTPS Server
	httpsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer httpsSrv.Close()

	// Client that trusts httptest self-signed cert
	validTLSClient := httpsSrv.Client()

	// Client that does NOT trust self-signed cert (strict TLS)
	strictTLSClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}

	t.Run("Explicit HTTP", func(t *testing.T) {
		var logBuf bytes.Buffer
		url, err := targets.AutoDetectTarget(ctx, validTLSClient, "http://example.com", &logBuf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if url != "http://example.com" {
			t.Errorf("got %q; want http://example.com", url)
		}
		if logBuf.Len() > 0 {
			t.Errorf("expected no logs for explicit scheme, got %q", logBuf.String())
		}
	})

	t.Run("Explicit HTTPS", func(t *testing.T) {
		var logBuf bytes.Buffer
		url, err := targets.AutoDetectTarget(ctx, validTLSClient, "https://example.com", &logBuf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if url != "https://example.com" {
			t.Errorf("got %q; want https://example.com", url)
		}
		if logBuf.Len() > 0 {
			t.Errorf("expected no logs for explicit scheme, got %q", logBuf.String())
		}
	})

	t.Run("Implicit HTTPS Success", func(t *testing.T) {
		var logBuf bytes.Buffer
		rawHost := strings.TrimPrefix(httpsSrv.URL, "https://")
		url, err := targets.AutoDetectTarget(ctx, validTLSClient, rawHost, &logBuf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantURL := "https://" + rawHost
		if url != wantURL {
			t.Errorf("got %q; want %q", url, wantURL)
		}
		logs := logBuf.String()
		if !strings.Contains(logs, "attempting HTTPS...") || !strings.Contains(logs, "Using HTTPS.") {
			t.Errorf("expected success log messages, got %q", logs)
		}
	})

	t.Run("Implicit HTTPS Fallback to HTTP", func(t *testing.T) {
		var logBuf bytes.Buffer
		rawHost := strings.TrimPrefix(httpSrv.URL, "http://")
		url, err := targets.AutoDetectTarget(ctx, validTLSClient, rawHost, &logBuf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantURL := "http://" + rawHost
		if url != wantURL {
			t.Errorf("got %q; want %q", url, wantURL)
		}
		logs := logBuf.String()
		if !strings.Contains(logs, "attempting HTTPS...") || !strings.Contains(logs, "HTTPS unavailable, falling back to HTTP.") {
			t.Errorf("expected fallback log messages, got %q", logs)
		}
	})

	t.Run("Invalid Certificate Does Not Fallback", func(t *testing.T) {
		var logBuf bytes.Buffer
		rawHost := strings.TrimPrefix(httpsSrv.URL, "https://")
		// Using strictTLSClient which fails certificate verification on httptest cert
		url, err := targets.AutoDetectTarget(ctx, strictTLSClient, rawHost, &logBuf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantURL := "https://" + rawHost
		if url != wantURL {
			t.Errorf("got %q; want %q (should NOT fall back to HTTP on invalid cert)", url, wantURL)
		}
		logs := logBuf.String()
		if strings.Contains(logs, "falling back to HTTP") {
			t.Errorf("did not expect fallback for certificate error, got logs: %q", logs)
		}
	})

	t.Run("IPv4 Host:Port Implicit Fallback", func(t *testing.T) {
		var logBuf bytes.Buffer
		rawHost := strings.TrimPrefix(httpSrv.URL, "http://")
		url, err := targets.AutoDetectTarget(ctx, validTLSClient, rawHost, &logBuf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(url, "http://") {
			t.Errorf("expected http:// prefix for plain HTTP server, got %q", url)
		}
	})

	t.Run("Quiet Mode Suppresses Informational Logs", func(t *testing.T) {
		rawHost := strings.TrimPrefix(httpSrv.URL, "http://")
		// Passing out = nil (simulating --quiet)
		url, err := targets.AutoDetectTarget(ctx, validTLSClient, rawHost, nil)
		if err != nil {
			t.Fatalf("unexpected error in quiet mode: %v", err)
		}
		wantURL := "http://" + rawHost
		if url != wantURL {
			t.Errorf("got %q; want %q", url, wantURL)
		}
	})
}
