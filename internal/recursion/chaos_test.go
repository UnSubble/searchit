package recursion_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/recursion"
)

type chaosRoundTripper struct {
	mode string
}

func (c *chaosRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	switch c.mode {
	case "timeout":
		time.Sleep(20 * time.Millisecond)
		return nil, context.DeadlineExceeded
	case "broken":
		return nil, errors.New("connection reset by peer")
	case "huge":
		// Return huge body stream
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("A"), 100000))),
		}, nil
	case "zerobyte":
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(nil)),
		}, nil
	case "invalid-headers":
		header := http.Header{}
		header["Invalid Header: Name"] = []string{"val1", "val2"}
		return &http.Response{
			StatusCode: 200,
			Header:     header,
			Body:       io.NopCloser(bytes.NewReader(nil)),
		}, nil
	case "duplicate-responses":
		// Simulates duplicate headers and values
		header := http.Header{}
		header.Add("Set-Cookie", "laravel_session=abc")
		header.Add("Set-Cookie", "laravel_session=def")
		return &http.Response{
			StatusCode: 200,
			Header:     header,
			Body:       io.NopCloser(bytes.NewReader(nil)),
		}, nil
	default:
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(nil)),
		}, nil
	}
}

func TestChaos_WorkerEdgeCases(t *testing.T) {
	modes := []string{"timeout", "broken", "huge", "zerobyte", "invalid-headers", "duplicate-responses"}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			cfg := config.Default()
			cfg.Recursive = true
			cfg.MaxDepth = 1

			rt := &chaosRoundTripper{mode: mode}
			client := &http.Client{Transport: rt}

			m := recursion.NewManager(
				client,
				cfg.Status.Exclude,
				testStaticReader{words: []string{"a", "b", "c"}},
				recursion.BFS,
				cfg.MaxDepth,
				cfg.RecurseOn,
				false,
				false,
				nil,
				nil,
				nil,
				nil,
				0,
				nil,
				nil,
			)
			m.SetDisableWildcard(true)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			resChan := m.Run(ctx, []string{"http://chaos.com"}, 4)
			var results []engine.Result
			for r := range resChan {
				results = append(results, r)
			}

			// Verify that regardless of broken responses/timeouts/huge body, the scheduler exits cleanly without lockups
			if mode == "broken" || mode == "timeout" {
				// No successful requests are accepted, so 0 results are emitted
				if len(results) != 0 {
					t.Errorf("Expected 0 results for mode %s, got %d: %+v", mode, len(results), results)
				}
			} else {
				// Success, root + 3 children = 4 results
				if len(results) != 4 {
					t.Errorf("Expected 4 results for mode %s, got %d: %+v", mode, len(results), results)
				}
			}
		})
	}
}

func TestChaos_HighScaleStarvationAndGrowth(t *testing.T) {
	cfg := config.Default()
	cfg.Recursive = true
	cfg.MaxDepth = 1

	rt := &chaosRoundTripper{mode: "zerobyte"}
	client := &http.Client{Transport: rt}

	// 100 targets seed list
	var targets []string
	for i := 0; i < 100; i++ {
		targets = append(targets, fmt.Sprintf("http://target-%d.com", i))
	}

	// Large wordlist to trigger frontier growth
	var words []string
	for i := 0; i < 1000; i++ {
		words = append(words, fmt.Sprintf("word-%d", i))
	}

	m := recursion.NewManager(
		client,
		cfg.Status.Exclude,
		testStaticReader{words: words},
		recursion.BFS,
		cfg.MaxDepth,
		cfg.RecurseOn,
		false,
		false,
		nil,
		nil,
		nil,
		nil,
		0,
		nil,
		nil,
	)
	m.SetDisableWildcard(true)

	// Stress-test with 128 workers
	resChan := m.Run(context.Background(), targets, 128)
	count := 0
	for range resChan {
		count++
	}

	// 100 targets + (100 targets * 1000 words) = 100,100 total jobs scanned
	expected := 100100
	if count != expected {
		t.Errorf("Expected exactly %d results, got %d", expected, count)
	}
}

func TestChaos_DeterministicWorkerMatrix(t *testing.T) {
	// Worker counts: 1, 2, 4, 8, 16, 32, 64, 128
	workerCounts := []int{1, 2, 4, 8, 16, 32, 64, 128}

	for _, w := range workerCounts {
		t.Run(fmt.Sprintf("Workers-%d", w), func(t *testing.T) {
			cfg := config.Default()
			cfg.Adaptive = true
			cfg.Recursive = true
			cfg.MaxDepth = 1

			rt := &mockRoundTripper{
				response: func(req *http.Request) (*http.Response, error) {
					header := http.Header{}
					if req.URL.Path == "" || req.URL.Path == "/" {
						header.Add("Set-Cookie", "laravel_session=abc123xyz")
					}
					return &http.Response{
						StatusCode: 200,
						Header:     header,
						Body:       io.NopCloser(bytes.NewReader(nil)),
					}, nil
				},
			}

			a := app.New(context.Background(), cfg)
			a.HTTPClient.Transport = app.WrapTransport(rt, a.FingerprintCache)

			m := recursion.NewManager(
				a.HTTPClient,
				cfg.Status.Exclude,
				testStaticReader{words: []string{"somepath"}},
				recursion.BFS,
				cfg.MaxDepth,
				cfg.RecurseOn,
				false,
				false,
				nil,
				nil,
				nil,
				nil,
				0,
				nil,
				a.FingerprintCache,
			)
			m.SetDisableWildcard(true)

			resChan := m.Run(context.Background(), []string{"http://determinism.com"}, w)
			var results []engine.Result
			for r := range resChan {
				results = append(results, r)
			}

			// Root (1) + wordlist path (1) + 5 injected paths = 7 results
			if len(results) != 7 {
				t.Errorf("Determinism violated under %d workers: expected 7 results, got %d", w, len(results))
			}
		})
	}
}
