package fingerprint_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/fingerprint"
)

func TestMatcher_Match_NoMatch(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")

	// Signals that are not tracked in rules
	fp.AddSignal(fingerprint.Signal{Source: "header:Server", Value: "nginx"})
	fp.AddSignal(fingerprint.Signal{Source: "html:link:stylesheet", Value: "/style.css"})

	matcher := fingerprint.NewMatcher()
	results := matcher.Match(fp)

	if len(results) != 0 {
		t.Errorf("Expected 0 technology matches, got %d: %+v", len(results), results)
	}
}

func TestMatcher_Match_LaravelSession(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")

	fp.AddSignal(fingerprint.Signal{Source: "cookie", Value: "laravel_session"})

	matcher := fingerprint.NewMatcher()
	results := matcher.Match(fp)

	if len(results) != 1 {
		t.Fatalf("Expected 1 technology match, got %d", len(results))
	}
	if results[0].Name != "Laravel" {
		t.Errorf("Expected technology Laravel, got %q", results[0].Name)
	}
}

func TestMatcher_Match_LaravelMultipleSignals(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")

	fp.AddSignal(fingerprint.Signal{Source: "cookie", Value: "laravel_session"})
	fp.AddSignal(fingerprint.Signal{Source: "html:script", Value: "/js/livewire.js?id=123"})
	fp.AddSignal(fingerprint.Signal{Source: "header:X-Powered-By", Value: "PHP/8.1"})

	matcher := fingerprint.NewMatcher()
	results := matcher.Match(fp)

	if len(results) != 1 {
		t.Fatalf("Expected 1 technology match, got %d", len(results))
	}
	if results[0].Name != "Laravel" {
		t.Errorf("Expected technology Laravel, got %q", results[0].Name)
	}
}

func TestMatcher_Match_LaravelExclusion(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")

	// Laravel session cookie but conflicting ASP.NET header excludes it
	fp.AddSignal(fingerprint.Signal{Source: "cookie", Value: "laravel_session"})
	fp.AddSignal(fingerprint.Signal{Source: "header:X-Powered-By", Value: "ASP.NET"})

	matcher := fingerprint.NewMatcher()
	results := matcher.Match(fp)

	for _, res := range results {
		if res.Name == "Laravel" {
			t.Fatal("Expected Laravel to be excluded due to conflicting ASP.NET header, but it matched")
		}
	}
}

func TestMatcher_Match_WordPress(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")

	fp.AddSignal(fingerprint.Signal{Source: "html:meta:name:generator", Value: "WordPress 5.8"})
	fp.AddSignal(fingerprint.Signal{Source: "html:link:stylesheet", Value: "/wp-content/themes/twentytwenty/style.css"})
	fp.AddSignal(fingerprint.Signal{Source: "cookie", Value: "wordpress_logged_in_xyz"})

	matcher := fingerprint.NewMatcher()
	results := matcher.Match(fp)

	if len(results) != 1 {
		t.Fatalf("Expected 1 technology match, got %d", len(results))
	}
	if results[0].Name != "WordPress" {
		t.Errorf("Expected technology WordPress, got %q", results[0].Name)
	}
}

func TestMatcher_Match_MultipleDetected(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")

	// WordPress assets + React data attribute → two matches
	fp.AddSignal(fingerprint.Signal{Source: "html:link:stylesheet", Value: "/wp-content/style.css"})
	fp.AddSignal(fingerprint.Signal{Source: "html:attr:data-reactroot", Value: ""})

	matcher := fingerprint.NewMatcher()
	results := matcher.Match(fp)

	if len(results) != 2 {
		t.Fatalf("Expected 2 technology matches, got %d: %+v", len(results), results)
	}

	names := make(map[string]bool)
	for _, res := range results {
		names[res.Name] = true
	}
	if !names["WordPress"] {
		t.Error("Expected WordPress to be detected")
	}
	if !names["React"] {
		t.Error("Expected React to be detected")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkMatcher_Evaluate(b *testing.B) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("bench.com")

	fp.AddSignal(fingerprint.Signal{Source: "header:Server", Value: "nginx/1.18"})
	fp.AddSignal(fingerprint.Signal{Source: "cookie", Value: "laravel_session"})
	fp.AddSignal(fingerprint.Signal{Source: "html:script", Value: "/js/livewire.js"})
	fp.AddSignal(fingerprint.Signal{Source: "header:X-Powered-By", Value: "PHP/8.1"})
	fp.AddSignal(fingerprint.Signal{Source: "html:attr:data-reactroot", Value: ""})
	fp.AddSignal(fingerprint.Signal{Source: "html:id", Value: "root"})

	matcher := fingerprint.NewMatcher()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = matcher.Match(fp)
	}
}
