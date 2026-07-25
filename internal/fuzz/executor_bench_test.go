package fuzz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/stats"
)

func BenchmarkExecute(b *testing.B) {
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := NewExecutor(ctx, client, fs, 50, 0, nil, collector, nil)
	defer e.Close()

	b.ResetTimer()

	// Simulate batch dispatch (what eager/dfs/bfs do)
	batchSize := 50
	for i := 0; i < b.N; i += batchSize {
		end := i + batchSize
		if end > b.N {
			end = b.N
		}

		var wg sync.WaitGroup
		for j := i; j < end; j++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				job := RequestDTO{
					URL:    srv.URL,
					Method: "GET",
				}
				_, _ = e.Execute(job)
			}(j)
		}
		wg.Wait()
	}
}
