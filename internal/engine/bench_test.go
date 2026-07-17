package engine_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
)

func benchApp(b *testing.B) *app.App {
	b.Helper()
	cfg := config.Default()
	cfg.Status.Exclude = nil
	return app.New(context.Background(), cfg)
}

func runBench(b *testing.B, workers int) {
	b.Helper()
	b.ReportAllocs()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := benchApp(b)
	fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	s := engine.NewScanner(a.HTTPClient, fs, nil, nil, 0, nil)

	urls := make([]string, b.N)
	for i := range urls {
		urls[i] = fmt.Sprintf("%s/%d", srv.URL, i)
	}

	b.ResetTimer()

	for range s.Scan(context.Background(), engine.SliceProducer{URLs: urls}, workers) {
	}
}

func BenchmarkWorker_RateLimitZeroOverhead(b *testing.B) {
	b.ReportAllocs()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := benchApp(b)
	fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	s := engine.NewScanner(a.HTTPClient, fs, nil, nil, 0, nil)

	urls := make([]string, b.N)
	for i := range urls {
		urls[i] = fmt.Sprintf("%s/%d", srv.URL, i)
	}

	b.ResetTimer()

	for range s.Scan(context.Background(), engine.SliceProducer{URLs: urls}, 32) {
	}
}

func BenchmarkWorkers_1(b *testing.B)   { runBench(b, 1) }
func BenchmarkWorkers_32(b *testing.B)  { runBench(b, 32) }
func BenchmarkWorkers_128(b *testing.B) { runBench(b, 128) }
