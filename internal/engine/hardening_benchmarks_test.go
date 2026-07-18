package engine_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/httpclient"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/recursion"
)

// BenchmarkFrontier measures the push/pop operations of the BFS/DFS recursion frontier.
func BenchmarkFrontier(b *testing.B) {
	b.ReportAllocs()
	f := recursion.NewFrontier(recursion.BFS)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Push(engine.Job{URL: "http://localhost/path", Depth: 1})
		_, _ = f.Pop()
	}
}

// BenchmarkProfileLoading measures profile store load performance.
func BenchmarkProfileLoading(b *testing.B) {
	b.ReportAllocs()
	store := profile.NewStore()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Load("scan/quick")
	}
}

// BenchmarkOutputFormatters measures serialization overhead of different output formatters.
func BenchmarkOutputFormatters(b *testing.B) {
	res := engine.Result{
		URL:        "https://example.com/admin/login.php?redirect=true",
		StatusCode: 200,
		Length:     4821,
		Depth:      2,
		Title:      "Administrator Login Panel",
		Headers:    http.Header{"Server": []string{"nginx/1.18.0"}},
	}

	b.Run("text", func(b *testing.B) {
		b.ReportAllocs()
		var buf bytes.Buffer
		fmttr := output.New(output.FormatText, &buf, false, true, true)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf.Reset()
			_ = fmttr.Print(res)
		}
	})

	b.Run("json", func(b *testing.B) {
		b.ReportAllocs()
		var buf bytes.Buffer
		fmttr := output.New(output.FormatJSON, &buf, false, true, true)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf.Reset()
			_ = fmttr.Print(res)
		}
	})

	b.Run("ndjson", func(b *testing.B) {
		b.ReportAllocs()
		var buf bytes.Buffer
		fmttr := output.New(output.FormatNDJSON, &buf, false, true, true)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf.Reset()
			_ = fmttr.Print(res)
		}
	})
}

// BenchmarkRedirectClientOverhead compares standard HTTP client to our redirect-handling client.
func BenchmarkRedirectClientOverhead(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b.Run("standard-client", func(b *testing.B) {
		b.ReportAllocs()
		c := &http.Client{Timeout: 10 * time.Second}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, err := c.Get(srv.URL)
			if err == nil {
				resp.Body.Close()
			}
		}
	})

	b.Run("redirect-client", func(b *testing.B) {
		b.ReportAllocs()
		c := httpclient.NewWithMaxRedirects(10*time.Second, 10*time.Second, true, 10, "")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, err := c.Get(srv.URL)
			if err == nil {
				resp.Body.Close()
			}
		}
	})
}

// BenchmarkRedirectChain measures processing time for redirects chains.
func BenchmarkRedirectChain(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch p {
		case "/1":
			w.Header().Set("Location", "/2")
			w.WriteHeader(http.StatusMovedPermanently)
		case "/2":
			w.Header().Set("Location", "/3")
			w.WriteHeader(http.StatusMovedPermanently)
		case "/3":
			w.Header().Set("Location", "/4")
			w.WriteHeader(http.StatusMovedPermanently)
		case "/4":
			w.Header().Set("Location", "/5")
			w.WriteHeader(http.StatusMovedPermanently)
		case "/5":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := httpclient.NewWithMaxRedirects(10*time.Second, 10*time.Second, true, 10, "")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := c.Get(srv.URL + "/1")
		if err == nil {
			resp.Body.Close()
		}
	}
}

// BenchmarkRedirectLoop measures how fast redirect loops are caught and terminated.
func BenchmarkRedirectLoop(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/a" {
			w.Header().Set("Location", "/b")
			w.WriteHeader(http.StatusFound)
		} else {
			w.Header().Set("Location", "/a")
			w.WriteHeader(http.StatusFound)
		}
	}))
	defer srv.Close()

	c := httpclient.NewWithMaxRedirects(10*time.Second, 10*time.Second, true, 10, "")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := c.Get(srv.URL + "/a")
		if err == nil {
			resp.Body.Close()
		}
	}
}
