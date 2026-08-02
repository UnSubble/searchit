package httpclient_test

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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

	httpclient.ConfigureTransportForWorkers(c, 128)
	if tr.MaxIdleConnsPerHost != 256 {
		t.Errorf("after ConfigureTransportForWorkers(128), MaxIdleConnsPerHost = %d, want 256", tr.MaxIdleConnsPerHost)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", tr.IdleConnTimeout)
	}
}

type dummyWrapper struct {
	underlying http.RoundTripper
}

func (d *dummyWrapper) RoundTrip(req *http.Request) (*http.Response, error) {
	return d.underlying.RoundTrip(req)
}

func (d *dummyWrapper) Unwrap() http.RoundTripper {
	return d.underlying
}

func TestConfigureTransportForWorkers_Wrapped(t *testing.T) {
	c := httpclient.New(10*time.Second, 3*time.Second, false, "")
	origTr := c.Transport.(*http.Transport)
	c.Transport = &dummyWrapper{underlying: c.Transport}

	httpclient.ConfigureTransportForWorkers(c, 128)
	if origTr.MaxIdleConnsPerHost != 256 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 256 for wrapped transport", origTr.MaxIdleConnsPerHost)
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
		t.Log("connection succeeded: skipping timeout validation (likely due to environment intercepting proxy)")
		return
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
	_ = httpclient.New(10*time.Second, 10*time.Second, false, ":\n")
}

func TestValidateHTTPVersion(t *testing.T) {
	valid := []string{"auto", "0.9", "1.0", "1.1", "2", "", "AUTO", " 1.1 "}
	for _, v := range valid {
		if err := httpclient.ValidateHTTPVersion(v); err != nil {
			t.Errorf("expected %q to be valid, got: %v", v, err)
		}
	}

	invalid := []string{"foo", "3", "h2", "HTTP/2", "tcp"}
	for _, v := range invalid {
		if err := httpclient.ValidateHTTPVersion(v); err == nil {
			t.Errorf("expected %q to be invalid, but validation passed", v)
		}
	}
}

func TestHTTPClient_HTTPVersion_Execution(t *testing.T) {
	readRequest := func(conn net.Conn) string {
		reader := bufio.NewReader(conn)
		var sb strings.Builder
		for {
			line, err := reader.ReadString('\n')
			sb.WriteString(line)
			if err != nil {
				break
			}
			// HTTP/0.9 has no protocol version on the request line (e.g. "GET /\r\n")
			// and ends immediately after line 1.
			if !strings.Contains(sb.String(), "HTTP/") {
				break
			}
			// HTTP/1.0+ headers end with an empty line.
			if line == "\r\n" || line == "\n" {
				break
			}
		}
		return sb.String()
	}

	t.Run("wire_format_0.9", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()

		go func() {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			wire := readRequest(conn)
			if !strings.HasPrefix(wire, "GET /\r\n") {
				t.Errorf("expected wire format GET /\\r\\n for HTTP/0.9, got %q", wire)
			}
			_, _ = conn.Write([]byte("HTTP/1.0 200 OK\r\nContent-Length: 5\r\nConnection: close\r\n\r\nHello"))
		}()

		c := httpclient.NewWithHTTPVersion(5*time.Second, 5*time.Second, false, 10, "", "0.9")
		resp, err := c.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		_ = resp.Body.Close()
	})

	t.Run("wire_format_1.0", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()

		go func() {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			wire := readRequest(conn)
			if !strings.HasPrefix(wire, "GET / HTTP/1.0\r\n") {
				t.Errorf("expected wire format GET / HTTP/1.0\\r\\n for HTTP/1.0, got %q", wire)
			}
			_, _ = conn.Write([]byte("HTTP/1.0 200 OK\r\nContent-Length: 5\r\nConnection: close\r\n\r\nHello"))
		}()

		c := httpclient.NewWithHTTPVersion(5*time.Second, 5*time.Second, false, 10, "", "1.0")
		resp, err := c.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		_ = resp.Body.Close()
	})

	t.Run("httptest_1.1_and_2", func(t *testing.T) {
		var capturedProto string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedProto = r.Proto
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		for _, ver := range []string{"1.1", "2", "auto"} {
			c := httpclient.NewWithHTTPVersion(5*time.Second, 5*time.Second, false, 10, "", ver)
			resp, err := c.Get(srv.URL)
			if err != nil {
				t.Fatalf("request failed for version %s: %v", ver, err)
			}
			_ = resp.Body.Close()
			if capturedProto != "HTTP/1.1" {
				t.Errorf("expected HTTP/1.1 on plain server for version %s, got %q", ver, capturedProto)
			}
		}
	})
}
