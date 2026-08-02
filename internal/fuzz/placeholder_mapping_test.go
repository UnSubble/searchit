package fuzz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/filter"
)

func runMappingTest(t *testing.T, r *Runner, primaryWords []string) []string {
	var mu sync.Mutex
	var requestedURLs []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		requestedURLs = append(requestedURLs, req.URL.String())
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	r.Client = ts.Client()
	if tr, ok := r.Client.Transport.(*http.Transport); ok {
		tr.DisableKeepAlives = true
	}
	r.FS = fs
	if r.Threads <= 0 {
		r.Threads = 2
	}
	if r.Method == "" {
		r.Method = "GET"
	}
	r.TargetURL = ts.URL + r.TargetURL

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var primaryChan chan string
	if primaryWords != nil {
		primaryChan = make(chan string, len(primaryWords))
		for _, w := range primaryWords {
			primaryChan <- w
		}
		close(primaryChan)
	}

	err := r.Run(ctx, ctx, "eager", primaryChan, func(res Result) {})
	if err != nil {
		t.Fatalf("unexpected error running runner: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	sort.Strings(requestedURLs)
	return requestedURLs
}

// 1. Test FUZZ only
func TestPlaceholderMapping_FUZZOnly(t *testing.T) {
	r := &Runner{
		TargetURL: "/FUZZ",
	}
	urls := runMappingTest(t, r, []string{"fuzz1", "fuzz2"})

	expected := []string{"/fuzz1", "/fuzz2"}
	if len(urls) != len(expected) {
		t.Fatalf("expected %d requests, got %d: %v", len(expected), len(urls), urls)
	}
	for i, u := range urls {
		if u != expected[i] {
			t.Errorf("request %d: got %s, want %s", i, u, expected[i])
		}
	}
}

// 2. Test FOO only
func TestPlaceholderMapping_FOOOnly(t *testing.T) {
	r := &Runner{
		TargetURL: "/FOO",
		FooWords:  []string{"foo1", "foo2"},
	}
	urls := runMappingTest(t, r, nil)

	expected := []string{"/foo1", "/foo2"}
	if len(urls) != len(expected) {
		t.Fatalf("expected %d requests, got %d: %v", len(expected), len(urls), urls)
	}
	for i, u := range urls {
		if u != expected[i] {
			t.Errorf("request %d: got %s, want %s", i, u, expected[i])
		}
	}
}

// 3. Test FUZZ + FOO together
func TestPlaceholderMapping_FUZZAndFOOTogether(t *testing.T) {
	r := &Runner{
		TargetURL: "/FUZZ/FOO",
		FooWords:  []string{"fooA", "fooB"},
	}
	urls := runMappingTest(t, r, []string{"fuzz1", "fuzz2"})

	expected := []string{
		"/fuzz1/fooA",
		"/fuzz1/fooB",
		"/fuzz2/fooA",
		"/fuzz2/fooB",
	}
	if len(urls) != len(expected) {
		t.Fatalf("expected %d requests, got %d: %v", len(expected), len(urls), urls)
	}
	for i, u := range urls {
		if u != expected[i] {
			t.Errorf("request %d: got %s, want %s", i, u, expected[i])
		}
	}
}

// 4. Test FUZZ + FOO + BAR together
func TestPlaceholderMapping_FUZZ_FOO_BAR(t *testing.T) {
	r := &Runner{
		TargetURL: "/FUZZ/FOO/BAR",
		FooWords:  []string{"o1"},
		BarWords:  []string{"r1"},
	}
	urls := runMappingTest(t, r, []string{"z1"})

	expected := []string{"/z1/o1/r1"}
	if len(urls) != len(expected) {
		t.Fatalf("expected %d requests, got %d: %v", len(expected), len(urls), urls)
	}
	if urls[0] != expected[0] {
		t.Errorf("got %s, want %s", urls[0], expected[0])
	}
}

// 5. Test Multiple Independent Placeholder Mappings (FUZZ, FOO, BAR, BAZ, BUZZ)
func TestPlaceholderMapping_AllPlaceholdersIndependent(t *testing.T) {
	r := &Runner{
		TargetURL: "/FUZZ/FOO/BAR/BAZ/BUZZ",
		FooWords:  []string{"fooVal"},
		BarWords:  []string{"barVal"},
		BazWords:  []string{"bazVal"},
		BuzzWords: []string{"buzzVal"},
	}
	urls := runMappingTest(t, r, []string{"fuzzVal"})

	expected := []string{"/fuzzVal/fooVal/barVal/bazVal/buzzVal"}
	if len(urls) != len(expected) {
		t.Fatalf("expected %d requests, got %d: %v", len(expected), len(urls), urls)
	}
	if urls[0] != expected[0] {
		t.Errorf("got %s, want %s", urls[0], expected[0])
	}
}

// 6. Regression Test: Prove --foo (FooWords) is NO LONGER overwritten by FUZZ primary wordlist
func TestPlaceholderMapping_Regression_FooNotOverwrittenByFUZZ(t *testing.T) {
	r := &Runner{
		TargetURL: "/FUZZ?param=FOO",
		FooWords:  []string{"explicit_foo"},
	}
	urls := runMappingTest(t, r, []string{"primary_fuzz"})

	// Before the fix, "FOO" was overwritten by "primary_fuzz", producing "/primary_fuzz?param=primary_fuzz"
	// After the fix, "FOO" retains "explicit_foo", producing "/primary_fuzz?param=explicit_foo"
	expected := []string{"/primary_fuzz?param=explicit_foo"}
	if len(urls) != len(expected) {
		t.Fatalf("expected %d requests, got %d: %v", len(expected), len(urls), urls)
	}
	if urls[0] != expected[0] {
		t.Errorf("REGRESSION DETECTED: got %s, want %s", urls[0], expected[0])
	}
}
