package httpclient_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/httpclient"
)

func TestNew_ReturnsClient(t *testing.T) {
	c := httpclient.New(10*time.Second, 10*time.Second, false, "")
	if c == nil {
		t.Fatal("New returned nil")
	}
}

func TestNew_TimeoutSet(t *testing.T) {
	c := httpclient.New(5*time.Second, 10*time.Second, false, "")
	if c.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", c.Timeout)
	}
}

func TestNew_HasTransport(t *testing.T) {
	c := httpclient.New(10*time.Second, 10*time.Second, false, "")
	if c.Transport == nil {
		t.Fatal("Transport is nil; connection pooling will be disabled")
	}
}

func TestNew_TransportSettings(t *testing.T) {
	c := httpclient.New(10*time.Second, 10*time.Second, false, "")
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}
	if tr.MaxIdleConns != 1000 {
		t.Errorf("MaxIdleConns = %d, want 1000", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 100 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 100", tr.MaxIdleConnsPerHost)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", tr.IdleConnTimeout)
	}
}

func TestContentLength_Present(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "42")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := httpclient.ContentLength(resp); got != 42 {
		t.Errorf("ContentLength = %d, want 42", got)
	}
}

func TestContentLength_Absent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter does not implement http.Flusher")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "chunk")
		flusher.Flush()
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := httpclient.ContentLength(resp); got != -1 {
		t.Errorf("ContentLength = %d, want -1 for chunked response without Content-Length", got)
	}
}

func TestNew_ConnectTimeout(t *testing.T) {
	c := httpclient.New(10*time.Second, 50*time.Millisecond, false, "")

	start := time.Now()
	_, err := c.Get("http://10.255.255.1:80")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected connection to fail due to timeout")
	}

	if elapsed > 2*time.Second {
		t.Errorf("expected connection attempt to time out quickly, but took %v", elapsed)
	}
}

func TestNew_FollowRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "/dest")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Destination"))
	}))
	defer srv.Close()

	t.Run("followRedirects=false", func(t *testing.T) {
		c := httpclient.New(5*time.Second, 5*time.Second, false, "")
		resp, err := c.Get(srv.URL + "/redirect")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusFound {
			t.Errorf("status code = %d, want 302", resp.StatusCode)
		}
	})

	t.Run("followRedirects=true", func(t *testing.T) {
		c := httpclient.New(5*time.Second, 5*time.Second, true, "")
		resp, err := c.Get(srv.URL + "/redirect")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status code = %d, want 200", resp.StatusCode)
		}
	})
}

func TestNew_Proxy(t *testing.T) {
	c := httpclient.New(10*time.Second, 10*time.Second, false, "http://127.0.0.1:8080")
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}
	if tr.Proxy == nil {
		t.Fatal("expected proxy configuration to be non-nil")
	}

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	proxyURL, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("proxy resolve failed: %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:8080" {
		t.Errorf("expected proxy URL http://127.0.0.1:8080, got %v", proxyURL)
	}
}

func TestNew_ProxyPanicOnInvalid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on invalid proxy URL format")
		}
	}()
	_ = httpclient.New(10*time.Second, 10*time.Second, false, "http://invalid-url::8080")
}
