package fingerprint

import (
	"io"
	"net/http"
	"testing"
)

type dummyBody struct {
	content []byte
}

func (d dummyBody) Read(p []byte) (n int, err error) {
	return copy(p, d.content), io.EOF
}

func (d dummyBody) Close() error {
	return nil
}

func BenchmarkProfile_HTMLDetector(b *testing.B) {
	htmlBody := []byte(`
<!DOCTYPE html>
<html>
<head>
    <meta name="generator" content="Laravel" />
    <meta charset="UTF-8">
    <link rel="stylesheet" href="css/app.css" />
    <script src="js/app.js"></script>
</head>
<body data-reactroot="true">
    <!-- generator: Laravel -->
    <div id="app"></div>
</body>
</html>
`)
	ctx := &Context{
		Host:       "bench.com",
		Path:       "/",
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/html"}},
		Body:       htmlBody,
	}

	b.Run("1-request", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			fp := newFingerprint("bench.com")
			detectHTML(ctx, fp)
		}
	})
}

func BenchmarkProfile_HeaderDetector(b *testing.B) {
	ctx := &Context{
		Host:       "bench.com",
		Path:       "/",
		StatusCode: 200,
		Header: http.Header{
			"Server":       []string{"nginx"},
			"X-Powered-By": []string{"PHP/8.1"},
			"Set-Cookie":   []string{"laravel_session=abc123xyz"},
		},
	}

	b.Run("1-request", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			fp := newFingerprint("bench.com")
			detectHeaders(ctx, fp)
		}
	})
}

func BenchmarkProfile_Matcher(b *testing.B) {
	fp := newFingerprint("bench.com")
	fp.AddSignal(Signal{Source: "cookie", Value: "laravel_session"})

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matcher := NewMatcher()
		_ = matcher.Match(fp)
	}
}

func BenchmarkProfile_Cache(b *testing.B) {
	cache := NewCache()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = cache.GetOrCreate("bench.com")
	}
}

func BenchmarkProfile_SignalInsertion(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		fp := newFingerprint("bench.com")
		fp.AddSignal(Signal{Source: "test", Value: "val"})
	}
}

func BenchmarkProfile_AdaptiveScale_Matrix(b *testing.B) {
	htmlBody := []byte(`
<!DOCTYPE html>
<html>
<head>
    <meta name="generator" content="Laravel" />
</head>
<body>
    <div id="app"></div>
</body>
</html>
`)
	ctx := &Context{
		Host:       "bench.com",
		Path:       "/",
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/html"}},
		Body:       htmlBody,
	}

	scales := []struct {
		name  string
		count int
	}{
		{"1-req", 1},
		{"100-req", 100},
		{"1k-req", 1000},
		{"10k-req", 10000},
		{"100k-req", 100000},
	}

	for _, sc := range scales {
		b.Run(sc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				cache := NewCache()
				engine := NewEngine(cache)
				for k := 0; k < sc.count; k++ {
					engine.Analyze(ctx)
				}
			}
		})
	}
}
