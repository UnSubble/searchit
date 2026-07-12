package fingerprint_test

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/unsubble/searchit/internal/fingerprint"
)

func TestEngine_Analyze_CreatesFingerprint(t *testing.T) {
	cache := fingerprint.NewCache()
	engine := fingerprint.NewEngine(cache)

	ctx := &fingerprint.Context{
		Host:       "example.com",
		Path:       "/index.html",
		StatusCode: 200,
		Header:     http.Header{"Server": []string{"nginx"}},
		Body:       []byte("Hello World"),
	}

	if fp := cache.Get("example.com"); fp != nil {
		t.Fatal("Expected no fingerprint for example.com initially, but one exists")
	}

	engine.Analyze(ctx)

	fp := cache.Get("example.com")
	if fp == nil {
		t.Fatal("Expected a fingerprint for example.com to be created, but got nil")
	}

	if fp.Host() != "example.com" {
		t.Errorf("Expected fingerprint host to be example.com, got %q", fp.Host())
	}
}

func TestEngine_Analyze_UsesExistingFingerprint(t *testing.T) {
	cache := fingerprint.NewCache()
	engine := fingerprint.NewEngine(cache)

	fpExisting := cache.GetOrCreate("example.com")
	fpExisting.AddSignal(fingerprint.Signal{
		Source:     "test",
		Value:      "pre-existing",
		Confidence: fingerprint.Confidence(1.0),
	})

	ctx := &fingerprint.Context{
		Host:       "example.com",
		Path:       "/",
		StatusCode: 200,
	}

	engine.Analyze(ctx)

	fp := cache.Get("example.com")
	if fp != fpExisting {
		t.Fatal("Expected engine to resolve and use the existing fingerprint instance, but got a different instance")
	}

	signals := fp.Signals()
	if len(signals) != 1 || signals[0].Value != "pre-existing" {
		t.Errorf("Expected fingerprint to retain pre-existing signals, got %+v", signals)
	}
}

func TestEngine_Analyze_Headers(t *testing.T) {
	tests := []struct {
		name        string
		header      http.Header
		wantSignals []fingerprint.Signal
	}{
		{
			name:        "Missing headers",
			header:      http.Header{},
			wantSignals: nil,
		},
		{
			name: "Standard headers",
			header: http.Header{
				"Server":       []string{"Apache/2.4.41"},
				"X-Powered-By": []string{"PHP/7.4.3"},
			},
			wantSignals: []fingerprint.Signal{
				{Source: "header:Server", Value: "Apache/2.4.41", Confidence: 1.0},
				{Source: "header:X-Powered-By", Value: "PHP/7.4.3", Confidence: 1.0},
			},
		},
		{
			name: "Case insensitivity and canonicalization",
			header: http.Header{
				"server":       []string{"nginx"},
				"x-powered-by": []string{"Next.js"},
			},
			wantSignals: []fingerprint.Signal{
				{Source: "header:Server", Value: "nginx", Confidence: 1.0},
				{Source: "header:X-Powered-By", Value: "Next.js", Confidence: 1.0},
			},
		},
		{
			name: "Multiple values on same header",
			header: http.Header{
				"Via": []string{"1.1 vegur", "1.0 squid"},
			},
			wantSignals: []fingerprint.Signal{
				{Source: "header:Via", Value: "1.1 vegur", Confidence: 1.0},
				{Source: "header:Via", Value: "1.0 squid", Confidence: 1.0},
			},
		},
		{
			name: "Empty values ignored",
			header: http.Header{
				"Server":       []string{"", "  ", "\t"},
				"X-Powered-By": []string{"Node.js"},
			},
			wantSignals: []fingerprint.Signal{
				{Source: "header:X-Powered-By", Value: "Node.js", Confidence: 1.0},
			},
		},
		{
			name: "Unrelated headers ignored",
			header: http.Header{
				"Server":         []string{"IIS"},
				"Date":           []string{"Sun, 12 Jul 2026 06:00:00 GMT"},
				"Content-Length": []string{"42"},
			},
			wantSignals: []fingerprint.Signal{
				{Source: "header:Server", Value: "IIS", Confidence: 1.0},
			},
		},
		{
			name: "Set-Cookie parsing simple",
			header: http.Header{
				"Set-Cookie": []string{"PHPSESSID=abc123xyz"},
			},
			wantSignals: []fingerprint.Signal{
				{Source: "cookie", Value: "PHPSESSID", Confidence: 1.0},
			},
		},
		{
			name: "Set-Cookie parsing complex options",
			header: http.Header{
				"Set-Cookie": []string{
					"laravel_session=xyz789; Path=/; HttpOnly; Secure; SameSite=Lax",
					"custom_cookie; Path=/; Secure",
				},
			},
			wantSignals: []fingerprint.Signal{
				{Source: "cookie", Value: "laravel_session", Confidence: 1.0},
				{Source: "cookie", Value: "custom_cookie", Confidence: 1.0},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := fingerprint.NewCache()
			engine := fingerprint.NewEngine(cache)

			ctx := &fingerprint.Context{
				Host:   "example.com",
				Header: tc.header,
			}

			engine.Analyze(ctx)

			fp := cache.Get("example.com")
			if fp == nil && len(tc.wantSignals) > 0 {
				t.Fatalf("Expected fingerprint to be created, got nil")
			}

			if fp == nil {
				return
			}

			gotSignals := fp.Signals()
			if len(gotSignals) != len(tc.wantSignals) {
				t.Fatalf("Signals count mismatch: got %d, want %d.\nGot: %+v\nWant: %+v",
					len(gotSignals), len(tc.wantSignals), gotSignals, tc.wantSignals)
			}

			// Validate values are matching
			matchMap := make(map[string]bool)
			for _, ws := range tc.wantSignals {
				matchMap[fmt.Sprintf("%s:%s", ws.Source, ws.Value)] = false
			}

			for _, gs := range gotSignals {
				key := fmt.Sprintf("%s:%s", gs.Source, gs.Value)
				if _, ok := matchMap[key]; !ok {
					t.Errorf("Unexpected signal recorded: %+v", gs)
					continue
				}
				matchMap[key] = true
			}

			for k, matched := range matchMap {
				if !matched {
					t.Errorf("Expected signal key %q was not found in fingerprint", k)
				}
			}
		})
	}
}

func TestEngine_Analyze_Concurrent(t *testing.T) {
	cache := fingerprint.NewCache()
	engine := fingerprint.NewEngine(cache)

	const workers = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				ctx := &fingerprint.Context{
					Host: "concurrent-host.com",
					Header: http.Header{
						"Server":     []string{fmt.Sprintf("Apache/worker-%d-%d", id, j)},
						"Set-Cookie": []string{fmt.Sprintf("cookie-%d; path=/", id)},
					},
				}
				engine.Analyze(ctx)
			}
		}(i)
	}

	wg.Wait()

	fp := cache.Get("concurrent-host.com")
	if fp == nil {
		t.Fatal("Expected fingerprint to be created under concurrent pressure, got nil")
	}

	// Each execution adds exactly 2 signals (1 server, 1 cookie).
	// Total expected: workers * iterations * 2
	expected := workers * iterations * 2
	got := len(fp.Signals())
	if got != expected {
		t.Errorf("Concurrent signals count mismatch: got %d, want %d", got, expected)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkHeaderDetector_Standard(b *testing.B) {
	cache := fingerprint.NewCache()
	engine := fingerprint.NewEngine(cache)

	ctx := &fingerprint.Context{
		Host: "bench.com",
		Header: http.Header{
			"Server":       []string{"nginx/1.18.0"},
			"X-Powered-By": []string{"Next.js"},
			"Via":          []string{"1.1 vegur"},
			"Set-Cookie":   []string{"JSESSIONID=abc123xyz; Path=/; Secure; HttpOnly"},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		engine.Analyze(ctx)
	}
}
