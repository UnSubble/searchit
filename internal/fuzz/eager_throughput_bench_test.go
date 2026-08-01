package fuzz

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/stats"
)

func BenchmarkEagerThroughput(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := srv.Client()
	tr := client.Transport.(*http.Transport)
	tr.MaxIdleConns = 1000
	tr.MaxIdleConnsPerHost = 1000

	fs, _ := filter.NewFilterSuite("", "", "", "", nil, nil, nil, nil)
	collector := stats.NewCollector()

	threads := 128

	runner := &Runner{
		TargetURL: srv.URL + "/FUZZ",
		Method:    "GET",
		Client:    client,
		FS:        fs,
		Threads:   threads,
		Collector: collector,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		primaryChan := make(chan string, 1000)
		numRequests := 10000

		go func() {
			defer close(primaryChan)
			for k := 0; k < numRequests; k++ {
				primaryChan <- fmt.Sprintf("word%d", k)
			}
		}()

		var recCount int64
		ctx, cancel := context.WithCancel(context.Background())
		start := time.Now()

		err := runner.Run(ctx, ctx, "eager", primaryChan, func(r Result) {
			atomic.AddInt64(&recCount, 1)
		})
		cancel()

		if err != nil {
			b.Fatalf("runner.Run failed: %v", err)
		}

		elapsed := time.Since(start)
		reqPerSec := float64(numRequests) / elapsed.Seconds()
		b.ReportMetric(reqPerSec, "req/s")
	}
}
