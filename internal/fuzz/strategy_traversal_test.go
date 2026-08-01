package fuzz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
)

func buildChan(words []string) <-chan string {
	ch := make(chan string, len(words))
	for _, w := range words {
		ch <- w
	}
	close(ch)
	return ch
}

func TestTraversalRegression(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)

	testCases := []struct {
		name          string
		strategy      string
		targetURL     string
		primaryWords  []string
		fooWords      []string
		buzzWords     []string
		expectedCount int
		expectedURLs  []string
	}{
		{
			name:          "DFS FUZZ + BUZZ using primaryChan",
			strategy:      "dfs",
			targetURL:     ts.URL + "/FUZZ/BUZZ",
			primaryWords:  []string{"a", "b"},
			fooWords:      nil,
			buzzWords:     []string{"1", "2"},
			expectedCount: 8,
			expectedURLs: []string{
				ts.URL + "/a",
				ts.URL + "/a",
				ts.URL + "/a/1",
				ts.URL + "/a/2",
				ts.URL + "/b",
				ts.URL + "/b",
				ts.URL + "/b/1",
				ts.URL + "/b/2",
			},
		},
		{
			name:          "BFS FUZZ + BUZZ using primaryChan",
			strategy:      "bfs",
			targetURL:     ts.URL + "/FUZZ/BUZZ",
			primaryWords:  []string{"a", "b"},
			fooWords:      nil,
			buzzWords:     []string{"1", "2"},
			expectedCount: 8,
			expectedURLs: []string{
				ts.URL + "/a",
				ts.URL + "/b",
				ts.URL + "/a",
				ts.URL + "/b",
				ts.URL + "/a/1",
				ts.URL + "/a/2",
				ts.URL + "/b/1",
				ts.URL + "/b/2",
			},
		},
		{
			name:          "DFS FOO + BUZZ using fooWords",
			strategy:      "dfs",
			targetURL:     ts.URL + "/FOO/BUZZ",
			primaryWords:  nil,
			fooWords:      []string{"c", "d"},
			buzzWords:     []string{"3", "4"},
			expectedCount: 8,
			expectedURLs: []string{
				ts.URL + "/c",
				ts.URL + "/c",
				ts.URL + "/c/3",
				ts.URL + "/c/4",
				ts.URL + "/d",
				ts.URL + "/d",
				ts.URL + "/d/3",
				ts.URL + "/d/4",
			},
		},
		{
			name:          "DFS fallback maxDepth == 0",
			strategy:      "dfs",
			targetURL:     ts.URL + "/plain",
			primaryWords:  []string{"x"},
			fooWords:      nil,
			buzzWords:     nil,
			expectedCount: 1,
			expectedURLs: []string{
				ts.URL + "/plain",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &fuzz.Runner{
				Client:    ts.Client(),
				Method:    "GET",
				TargetURL: tc.targetURL,
				Threads:   1,
				Delay:     0,
				FS:        fs,
				FooWords:  tc.fooWords,
				BuzzWords: tc.buzzWords,
			}

			var primaryChan <-chan string
			if tc.primaryWords != nil {
				primaryChan = buildChan(tc.primaryWords)
			}

			var results []string
			yield := func(res fuzz.Result) {
				if res.Accepted {
					results = append(results, res.URL)
				}
			}

			err := r.Run(context.Background(), context.Background(), tc.strategy, primaryChan, yield)
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}

			if len(results) != tc.expectedCount {
				t.Fatalf("Expected %d results, got %d", tc.expectedCount, len(results))
			}

			if len(tc.expectedURLs) > 0 {
				if tc.strategy == "dfs" || tc.strategy == "bfs" {
					for i, expected := range tc.expectedURLs {
						if i < len(results) && results[i] != expected {
							t.Errorf("At index %d: expected %q, got %q", i, expected, results[i])
						}
					}
				} else {
					sort.Strings(results)
					sort.Strings(tc.expectedURLs)
					for i, expected := range tc.expectedURLs {
						if results[i] != expected {
							t.Errorf("At index %d: expected %q, got %q", i, expected, results[i])
						}
					}
				}
			}
		})
	}
}
