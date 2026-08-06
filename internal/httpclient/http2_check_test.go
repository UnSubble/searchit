package httpclient

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

func TestHTTP2Check(t *testing.T) {
	// Create a test server that supports HTTP/2
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Proto: %s", r.Proto)
	}))

	// Configure HTTP/2 on the server
	err := http2.ConfigureServer(srv.Config, nil)
	if err != nil {
		t.Fatalf("Failed to configure http2 server: %v", err)
	}

	srv.TLS = srv.Config.TLSConfig
	srv.StartTLS()
	defer srv.Close()

	t.Logf("Server running on: %s", srv.URL)

	// Now check if our New client automatically negotiates HTTP/2
	client := New(Options{
		Timeout:        10 * time.Second,
		ConnectTimeout: 2 * time.Second,
	})

	// Set insecure skip verify
	if tr, ok := client.Transport.(*http.Transport); ok {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Error requesting server: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("Response Body: %s", string(body))
	t.Logf("Response Proto: %s", resp.Proto)
}

func TestHTTPClient_HTTPVersion_ForcedHTTP11_OnH2Server(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s", r.Proto)
	}))
	_ = http2.ConfigureServer(srv.Config, nil)
	srv.TLS = srv.Config.TLSConfig
	srv.StartTLS()
	defer srv.Close()

	// Client with httpVersion = "1.1"
	client := New(Options{
		Timeout:        5 * time.Second,
		ConnectTimeout: 2 * time.Second,
		HTTPVersion:    "1.1",
	})
	if tr, ok := client.Transport.(*http.Transport); ok {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "HTTP/1.1" {
		t.Errorf("expected HTTP/1.1 when forced, got %q", string(body))
	}
}
