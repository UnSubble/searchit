package fuzz_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/fuzz"
)

func TestFuzz_ConcurrencyAndDeterminism(t *testing.T) {
	// Matrix of worker counts: 1, 2, 4, 8, 16, 32, 64
	threadCounts := []int{1, 2, 4, 8, 16, 32, 64}

	// Permutations: 3 (foo) * 3 (bar) = 9 jobs
	fooWords := []string{"foo1", "foo2", "foo3"}
	barWords := []string{"bar1", "bar2", "bar3"}

	var mu sync.Mutex
	requestCounts := make(map[string]int)

	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			requestCounts[req.URL.String()]++
			mu.Unlock()

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	for _, tc := range threadCounts {
		t.Run(fmt.Sprintf("Threads-%d", tc), func(t *testing.T) {
			mu.Lock()
			// Reset telemetry records
			requestCounts = make(map[string]int)
			mu.Unlock()

			ctx := context.Background()

			generator := fuzz.NewGenerator(
				"http://test.com/api?user=FOO&id=BAR",
				"GET",
				"",
				nil,
				"",
				fooWords,
				barWords,
				nil,
			)

			jobs := make(chan fuzz.Job, tc)
			go func() {
				defer close(jobs)
				generator.Generate(ctx, nil, jobs)
			}()

			results := fuzz.Start(
				ctx,
				client,
				nil,
				nil,
				nil,
				tc,
				0,
				nil,
				jobs,
				nil,
			)

			var res []fuzz.Result
			for r := range results {
				res = append(res, r)
			}

			// Verify count is exactly 9
			if len(res) != 9 {
				t.Fatalf("expected 9 results, got %d", len(res))
			}

			// Verify that every single permutation was fuzzed EXACTLY once (determinism)
			mu.Lock()
			defer mu.Unlock()
			if len(requestCounts) != 9 {
				t.Errorf("expected 9 unique URLs requested, got %d", len(requestCounts))
			}
			for url, count := range requestCounts {
				if count != 1 {
					t.Errorf("URL %q requested %d times, expected exactly 1", url, count)
				}
			}
		})
	}
}

func TestFuzz_TimeoutAndCancellation(t *testing.T) {
	rt := &mockRoundTripper{
		response: func(req *http.Request) (*http.Response, error) {
			time.Sleep(50 * time.Millisecond) // Simulated latency
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}
	client := &http.Client{Transport: rt}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	generator := fuzz.NewGenerator(
		"http://test.com/api?id=FUZZ",
		"GET",
		"",
		nil,
		"",
		nil,
		nil,
		nil,
	)

	primaryChan := make(chan string, 10)
	for i := 0; i < 10; i++ {
		primaryChan <- fmt.Sprintf("val-%d", i)
	}
	close(primaryChan)

	jobs := make(chan fuzz.Job, 4)
	go func() {
		defer close(jobs)
		generator.Generate(ctx, primaryChan, jobs)
	}()

	results := fuzz.Start(
		ctx,
		client,
		nil,
		nil,
		nil,
		4,
		0,
		nil,
		jobs,
		nil,
	)

	var res []fuzz.Result
	for r := range results {
		res = append(res, r)
	}

	// Should have exited cleanly without locking up, and some jobs must have failed/cancelled
	for _, r := range res {
		if !r.Accepted && r.Err == nil {
			t.Errorf("rejected result without error details: %+v", r)
		}
	}
}
