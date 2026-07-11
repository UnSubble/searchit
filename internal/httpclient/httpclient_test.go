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
	c := httpclient.New(10*time.Second, 10*time.Second)
	if c == nil {
		t.Fatal("New returned nil")
	}
}

func TestNew_TimeoutSet(t *testing.T) {
	c := httpclient.New(5*time.Second, 10*time.Second)
	if c.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", c.Timeout)
	}
}

func TestNew_HasTransport(t *testing.T) {
	c := httpclient.New(10*time.Second, 10*time.Second)
	if c.Transport == nil {
		t.Fatal("Transport is nil; connection pooling will be disabled")
	}
}

func TestNew_TransportSettings(t *testing.T) {
	c := httpclient.New(10*time.Second, 10*time.Second)
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
	// Force chunked transfer encoding by flushing incrementally; the server
	// cannot know the total length ahead of time, so it omits Content-Length.
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
	c := httpclient.New(10*time.Second, 50*time.Millisecond)

	// Use an unroutable IP address that will time out during TCP connection establishment
	start := time.Now()
	_, err := c.Get("http://10.255.255.1:80")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected connection to fail due to timeout")
	}

	// It should fail quickly (well under the 10s request timeout)
	if elapsed > 2*time.Second {
		t.Errorf("expected connection attempt to time out quickly, but took %v", elapsed)
	}
}
