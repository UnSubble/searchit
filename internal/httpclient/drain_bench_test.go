package httpclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBenchmark_ConnectionReuseMatrix(t *testing.T) {
	sizes := []struct {
		name string
		size int
	}{
		{"512 B (Tiny)", 512},
		{"4 KB (Standard 404)", 4 * 1024},
		{"16 KB (Medium Error)", 16 * 1024},
		{"32 KB (Large Error)", 32 * 1024},
		{"64 KB (Boundary)", 64 * 1024},
		{"128 KB (Oversized)", 128 * 1024},
		{"1 MB (Huge File)", 1024 * 1024},
	}

	fmt.Println("\n================================================================================")
	fmt.Println("              HTTP CONNECTION REUSE & DRAIN BENCHMARK (64 KB)")
	fmt.Println("================================================================================")

	for _, s := range sizes {
		payload := make([]byte, s.size)
		for i := range payload {
			payload[i] = 'X'
		}

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
			w.WriteHeader(http.StatusNotFound)
			w.Write(payload)
		}))

		totalRequests := 5000
		concurrency := 32

		client := New(Options{
			MaxWorkers: concurrency,
		})

		var tcpConns int64
		var reusedConns int64
		var completedReqs int64

		jobs := make(chan struct{}, totalRequests)
		for i := 0; i < totalRequests; i++ {
			jobs <- struct{}{}
		}
		close(jobs)

		start := time.Now()
		var wg sync.WaitGroup

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range jobs {
					trace := &httptrace.ClientTrace{
						GotConn: func(info httptrace.GotConnInfo) {
							if info.Reused {
								atomic.AddInt64(&reusedConns, 1)
							} else {
								atomic.AddInt64(&tcpConns, 1)
							}
						},
					}

					req, err := http.NewRequestWithContext(
						httptrace.WithClientTrace(context.Background(), trace),
						http.MethodGet,
						ts.URL,
						nil,
					)
					if err != nil {
						continue
					}

					resp, err := client.Do(req)
					if err != nil {
						continue
					}

					DrainAndCloseResponse(resp)
					atomic.AddInt64(&completedReqs, 1)
				}
			}()
		}

		wg.Wait()
		duration := time.Since(start)
		rps := float64(completedReqs) / duration.Seconds()
		reusePct := float64(reusedConns) / float64(completedReqs) * 100.0

		fmt.Printf("%-24s | Reused: %5.1f%% | TCP Conns: %4d | Reused Conns: %5d | %8.0f req/s\n",
			s.name, reusePct, atomic.LoadInt64(&tcpConns), atomic.LoadInt64(&reusedConns), rps)

		ts.Close()
	}
	fmt.Println("================================================================================")
}
