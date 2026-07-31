package fuzz

import (
	"context"
	"net/http"
	"strings"
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
		nil,
	)

	b.ResetTimer()
	gen.Generate(context.Background(), words, jobs)
}

// ==========================================
// Placeholder Rendering Benchmarks
// ==========================================

const benchTemplate = "http://example.com/api/FUZZ/v1/FOO/test?session=BAR&baz=BAZ&tracking=BUZZ&id=FUZZ"

var benchValues = [5]string{"fuzz_value", "foo_value", "bar_value", "baz_value", "buzz_value"}

func BenchmarkReplaceAll(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := benchTemplate
		res = strings.ReplaceAll(res, "FUZZ", benchValues[PlaceholderFUZZ])
		res = strings.ReplaceAll(res, "FOO", benchValues[PlaceholderFOO])
		res = strings.ReplaceAll(res, "BAR", benchValues[PlaceholderBAR])
		res = strings.ReplaceAll(res, "BAZ", benchValues[PlaceholderBAZ])
		res = strings.ReplaceAll(res, "BUZZ", benchValues[PlaceholderBUZZ])
		_ = res
	}
}

func BenchmarkBuilder(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		template := benchTemplate
		fuzzVal := benchValues[PlaceholderFUZZ]
		fooVal := benchValues[PlaceholderFOO]
		barVal := benchValues[PlaceholderBAR]
		bazVal := benchValues[PlaceholderBAZ]
		buzzVal := benchValues[PlaceholderBUZZ]

		fuzzCount, fooCount, barCount, bazCount, buzzCount := 0, 0, 0, 0, 0
		j := 0
		for j < len(template) {
			if template[j] == 'F' {
				if strings.HasPrefix(template[j:], "FUZZ") {
					fuzzCount++
					j += 4
					continue
				}
				if strings.HasPrefix(template[j:], "FOO") {
					fooCount++
					j += 3
					continue
				}
			} else if template[j] == 'B' {
				if strings.HasPrefix(template[j:], "BAR") {
					barCount++
					j += 3
					continue
				}
				if strings.HasPrefix(template[j:], "BAZ") {
					bazCount++
					j += 3
					continue
				}
				if strings.HasPrefix(template[j:], "BUZZ") {
					buzzCount++
					j += 4
					continue
				}
			}
			j++
		}

		finalLen := len(template) +
			fuzzCount*(len(fuzzVal)-4) +
			fooCount*(len(fooVal)-3) +
			barCount*(len(barVal)-3) +
			bazCount*(len(bazVal)-3) +
			buzzCount*(len(buzzVal)-4)

		var builder strings.Builder
		builder.Grow(finalLen)

		j = 0
		for j < len(template) {
			if template[j] == 'F' {
				if strings.HasPrefix(template[j:], "FUZZ") {
					builder.WriteString(fuzzVal)
					j += 4
					continue
				}
				if strings.HasPrefix(template[j:], "FOO") {
					builder.WriteString(fooVal)
					j += 3
					continue
				}
			} else if template[j] == 'B' {
				if strings.HasPrefix(template[j:], "BAR") {
					builder.WriteString(barVal)
					j += 3
					continue
				}
				if strings.HasPrefix(template[j:], "BAZ") {
					builder.WriteString(bazVal)
					j += 3
					continue
				}
				if strings.HasPrefix(template[j:], "BUZZ") {
					builder.WriteString(buzzVal)
					j += 4
					continue
				}
			}
			builder.WriteByte(template[j])
			j++
		}
		_ = builder.String()
	}
}

func BenchmarkCompiledTemplate(b *testing.B) {
	compiled := CompileGenTemplate(benchTemplate)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = compiled.Render(benchValues)
	}
}
