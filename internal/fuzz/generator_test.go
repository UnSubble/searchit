package fuzz_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/unsubble/searchit/internal/fuzz"
)

func TestGenerator_SingleFuzz(t *testing.T) {
	g := fuzz.NewGenerator(
		"http://test.com/FUZZ",
		"GET",
		"",
		nil,
		"",
		nil,
		nil,
		nil,
	)

	primaryChan := make(chan string, 3)
	primaryChan <- "a"
	primaryChan <- "b"
	primaryChan <- "c"
	close(primaryChan)

	jobs := make(chan fuzz.Job, 10)
	g.Generate(context.Background(), primaryChan, jobs)
	close(jobs)

	var results []fuzz.Job
	for j := range jobs {
		results = append(results, j)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(results))
	}
	expectedURLs := []string{
		"http://test.com/a",
		"http://test.com/b",
		"http://test.com/c",
	}
	for i, r := range results {
		if r.URL != expectedURLs[i] {
			t.Errorf("expected URL %q, got %q", expectedURLs[i], r.URL)
		}
		if r.Method != "GET" {
			t.Errorf("expected Method GET, got %q", r.Method)
		}
	}
}

func TestGenerator_CartesianProduct(t *testing.T) {
	g := fuzz.NewGenerator(
		"http://FOO.test.com/BAR",
		"POST",
		"data=BUZZ",
		http.Header{"X-Header": []string{"FUZZ"}},
		"",
		[]string{"foo1", "foo2"},
		[]string{"bar1", "bar2", "bar3"},
		[]string{"buzz1"},
	)

	primaryChan := make(chan string, 2)
	primaryChan <- "fuzz1"
	primaryChan <- "fuzz2"
	close(primaryChan)

	jobs := make(chan fuzz.Job, 20)
	g.Generate(context.Background(), primaryChan, jobs)
	close(jobs)

	var results []fuzz.Job
	for j := range jobs {
		results = append(results, j)
	}

	// Permutations: 2 (fuzz) * 2 (foo) * 3 (bar) * 1 (buzz) = 12 jobs
	if len(results) != 12 {
		t.Fatalf("expected 12 jobs, got %d", len(results))
	}

	// Verify the Cartesian ordering (nesting order: fuzz -> foo -> bar -> buzz)
	expectedFirst := fuzz.Job{
		URL:     "http://foo1.test.com/bar1",
		Method:  "POST",
		Body:    []byte("data=buzz1"),
		Headers: http.Header{"X-Header": []string{"fuzz1"}},
	}
	first := results[0]
	if first.URL != expectedFirst.URL {
		t.Errorf("expected URL %q, got %q", expectedFirst.URL, first.URL)
	}
	if string(first.Body) != string(expectedFirst.Body) {
		t.Errorf("expected Body %q, got %q", string(expectedFirst.Body), string(first.Body))
	}
	if first.Headers.Get("X-Header") != "fuzz1" {
		t.Errorf("expected Header value fuzz1, got %q", first.Headers.Get("X-Header"))
	}
}

func TestGenerator_NoPrimaryWordlist(t *testing.T) {
	g := fuzz.NewGenerator(
		"http://test.com/api?user=FOO&id=BAR",
		"GET",
		"",
		nil,
		"",
		[]string{"admin", "user"},
		[]string{"1", "2"},
		nil,
	)

	jobs := make(chan fuzz.Job, 10)
	g.Generate(context.Background(), nil, jobs)
	close(jobs)

	var results []fuzz.Job
	for j := range jobs {
		results = append(results, j)
	}

	// Permutations: 2 (foo) * 2 (bar) = 4 jobs
	if len(results) != 4 {
		t.Fatalf("expected 4 jobs, got %d", len(results))
	}

	expectedURLs := []string{
		"http://test.com/api?user=admin&id=1",
		"http://test.com/api?user=admin&id=2",
		"http://test.com/api?user=user&id=1",
		"http://test.com/api?user=user&id=2",
	}
	for i, r := range results {
		if r.URL != expectedURLs[i] {
			t.Errorf("expected URL %q, got %q", expectedURLs[i], r.URL)
		}
	}
}
