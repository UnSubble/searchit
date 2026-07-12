package fingerprint_test

import (
	"math"
	"testing"

	"github.com/unsubble/searchit/internal/fingerprint"
)

// Helper to check float equivalence
func almostEqual(a, b float32) bool {
	return math.Abs(float64(a-b)) < 0.0001
}

func TestMatcher_Match_NoMatch(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")

	// Signals that are not tracked in rules
	fp.AddSignal(fingerprint.Signal{Source: "header:Server", Value: "nginx", Confidence: 1.0})
	fp.AddSignal(fingerprint.Signal{Source: "html:link:stylesheet", Value: "/style.css", Confidence: 1.0})

	matcher := fingerprint.NewMatcher()
	results := matcher.Match(fp)

	if len(results) != 0 {
		t.Errorf("Expected 0 technology matches, got %d: %+v", len(results), results)
	}
}

func TestMatcher_Match_SingleSignal(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")

	// Single Laravel session cookie
	fp.AddSignal(fingerprint.Signal{Source: "cookie", Value: "laravel_session", Confidence: 1.0})

	matcher := fingerprint.NewMatcher()
	results := matcher.Match(fp)

	if len(results) != 1 {
		t.Fatalf("Expected 1 technology match, got %d", len(results))
	}

	res := results[0]
	if res.Name != "Laravel" {
		t.Errorf("Expected technology Laravel, got %q", res.Name)
	}

	expectedConf := fingerprint.Confidence(0.9)
	if !almostEqual(float32(res.Confidence), float32(expectedConf)) {
		t.Errorf("Expected Laravel confidence %v, got %v", expectedConf, res.Confidence)
	}
}

func TestMatcher_Match_MultipleSignals(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")

	// Laravel session cookie + livewire script + weak PHP server indicator
	fp.AddSignal(fingerprint.Signal{Source: "cookie", Value: "laravel_session", Confidence: 1.0})
	fp.AddSignal(fingerprint.Signal{Source: "html:script", Value: "/js/livewire.js?id=123", Confidence: 1.0})
	fp.AddSignal(fingerprint.Signal{Source: "header:X-Powered-By", Value: "PHP/8.1", Confidence: 1.0})

	matcher := fingerprint.NewMatcher()
	results := matcher.Match(fp)

	if len(results) != 1 {
		t.Fatalf("Expected 1 technology match, got %d", len(results))
	}

	res := results[0]
	if res.Name != "Laravel" {
		t.Errorf("Expected technology Laravel, got %q", res.Name)
	}

	// 1 - (1 - 0.9) * (1 - 0.8) * (1 - 0.15)
	// 1 - 0.1 * 0.2 * 0.85 = 1 - 0.017 = 0.983
	expectedConf := fingerprint.Confidence(0.983)
	if !almostEqual(float32(res.Confidence), float32(expectedConf)) {
		t.Errorf("Expected Laravel confidence %v, got %v", expectedConf, res.Confidence)
	}
}

func TestMatcher_Match_WordPressAlgebraicSum(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")

	// WordPress generator + stylesheet link + logged-in cookie
	fp.AddSignal(fingerprint.Signal{Source: "html:meta:name:generator", Value: "WordPress 5.8", Confidence: 1.0})
	fp.AddSignal(fingerprint.Signal{Source: "html:link:stylesheet", Value: "/wp-content/themes/twentytwenty/style.css", Confidence: 1.0})
	fp.AddSignal(fingerprint.Signal{Source: "cookie", Value: "wordpress_logged_in_xyz", Confidence: 1.0})

	matcher := fingerprint.NewMatcher()
	results := matcher.Match(fp)

	if len(results) != 1 {
		t.Fatalf("Expected 1 technology match, got %d", len(results))
	}

	res := results[0]
	if res.Name != "WordPress" {
		t.Errorf("Expected technology WordPress, got %q", res.Name)
	}

	// 1 - (1 - 0.95) * (1 - 0.9) * (1 - 0.9)
	// 1 - 0.05 * 0.1 * 0.1 = 1 - 0.0005 = 0.9995
	expectedConf := fingerprint.Confidence(0.9995)
	if !almostEqual(float32(res.Confidence), float32(expectedConf)) {
		t.Errorf("Expected WordPress confidence %v, got %v", expectedConf, res.Confidence)
	}
}

func TestMatcher_Match_ConflictingAndMultiple(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")

	// WordPress theme stylesheet link + React data attribute
	fp.AddSignal(fingerprint.Signal{Source: "html:link:stylesheet", Value: "/wp-content/style.css", Confidence: 1.0})
	fp.AddSignal(fingerprint.Signal{Source: "html:attr:data-reactroot", Value: "", Confidence: 1.0})

	matcher := fingerprint.NewMatcher()
	results := matcher.Match(fp)

	if len(results) != 2 {
		t.Fatalf("Expected 2 technology matches, got %d: %+v", len(results), results)
	}

	matches := make(map[string]float32)
	for _, res := range results {
		matches[res.Name] = float32(res.Confidence)
	}

	if wpConf, ok := matches["WordPress"]; !ok || !almostEqual(wpConf, 0.9) {
		t.Errorf("WordPress match wrong: ok=%v, conf=%v", ok, wpConf)
	}

	if reactConf, ok := matches["React"]; !ok || !almostEqual(reactConf, 0.95) {
		t.Errorf("React match wrong: ok=%v, conf=%v", ok, reactConf)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkMatcher_Evaluate(b *testing.B) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("bench.com")

	// Hydrate fingerprint with 10 typical signals
	fp.AddSignal(fingerprint.Signal{Source: "header:Server", Value: "nginx/1.18", Confidence: 1.0})
	fp.AddSignal(fingerprint.Signal{Source: "cookie", Value: "laravel_session", Confidence: 1.0})
	fp.AddSignal(fingerprint.Signal{Source: "html:script", Value: "/js/livewire.js", Confidence: 1.0})
	fp.AddSignal(fingerprint.Signal{Source: "header:X-Powered-By", Value: "PHP/8.1", Confidence: 1.0})
	fp.AddSignal(fingerprint.Signal{Source: "html:attr:data-reactroot", Value: "", Confidence: 1.0})
	fp.AddSignal(fingerprint.Signal{Source: "html:id", Value: "root", Confidence: 1.0})

	matcher := fingerprint.NewMatcher()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = matcher.Match(fp)
	}
}
