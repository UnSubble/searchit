package fuzz

import (
	"context"
	"net/http"
	"testing"
)

func BenchmarkGenerator(b *testing.B) {
	// Create a dummy generator
	words := make(chan string, b.N)
	for i := 0; i < b.N; i++ {
		words <- "testword"
	}
	close(words)

	jobs := make(chan RequestDTO, b.N)
	headers := make(http.Header)
	headers.Add("X-Test", "FOO")

	gen := NewGenerator(
		"http://example.com/FUZZ",
		"GET",
		"body=BAR",
		headers,
		"session=BUZZ",
		nil,
		nil,
		nil,
	)

	b.ResetTimer()
	gen.Generate(context.Background(), words, jobs)
}
