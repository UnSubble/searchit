package fuzz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/stats"
)

func BenchmarkFuzzPipeline(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport.(*http.Transport).MaxIdleConnsPerHost = 100
	client.Transport.(*http.Transport).MaxIdleConns = 100

	fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	collector := stats.NewCollector()

	jobs := make(chan WorkItem, b.N)
	results := make(chan Result, b.N)

	for i := 0; i < b.N; i++ {
		jobs <- WorkItem{
			Req: RequestDTO{
				URL:    srv.URL,
				Method: "GET",
			},
		}
	}
	close(jobs)

	b.ResetTimer()

	for i := 0; i < 50; i++ {
		go Worker(context.Background(), client, fs, 0, nil, jobs, results, collector, nil)
	}

	var count int32
	for i := 0; i < b.N; i++ {
		<-results
		atomic.AddInt32(&count, 1)
	}
}
