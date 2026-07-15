package fingerprint_test

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
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
		Source: "test",
		Value:  "pre-existing",
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
				{Source: "header:Server", Value: "Apache/2.4.41"},
				{Source: "header:X-Powered-By", Value: "PHP/7.4.3"},
			},
		},
		{
			name: "Case insensitivity and canonicalization",
			header: http.Header{
				"server":       []string{"nginx"},
				"x-powered-by": []string{"Next.js"},
			},
			wantSignals: []fingerprint.Signal{
				{Source: "header:Server", Value: "nginx"},
				{Source: "header:X-Powered-By", Value: "Next.js"},
			},
		},
		{
			name: "Multiple values on same header",
			header: http.Header{
				"Via": []string{"1.1 vegur", "1.0 squid"},
			},
			wantSignals: []fingerprint.Signal{
				{Source: "header:Via", Value: "1.1 vegur"},
				{Source: "header:Via", Value: "1.0 squid"},
			},
		},
		{
			name: "Empty values ignored",
			header: http.Header{
				"Server":       []string{"", "  ", "\t"},
				"X-Powered-By": []string{"Node.js"},
			},
			wantSignals: []fingerprint.Signal{
				{Source: "header:X-Powered-By", Value: "Node.js"},
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
				{Source: "header:Server", Value: "IIS"},
			},
		},
		{
			name: "Set-Cookie parsing simple",
			header: http.Header{
				"Set-Cookie": []string{"PHPSESSID=abc123xyz"},
			},
			wantSignals: []fingerprint.Signal{
				{Source: "cookie", Value: "PHPSESSID"},
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
				{Source: "cookie", Value: "laravel_session"},
				{Source: "cookie", Value: "custom_cookie"},
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

func TestEngine_Analyze_HTML(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		path        string
		body        string
		wantSignals []fingerprint.Signal
	}{
		{
			name:        "Non-HTML response ignored",
			contentType: "application/json",
			body:        `{"name": "generator", "content": "WP"}`,
			wantSignals: nil,
		},
		{
			name:        "HTML fallback by path extension",
			contentType: "",
			path:        "/home.html",
			body:        `<meta name="generator" content="WP">`,
			wantSignals: []fingerprint.Signal{
				{Source: "html:meta:name:generator", Value: "WP"},
			},
		},
		{
			name:        "Standard valid HTML parsing",
			contentType: "text/html",
			body: `
				<!DOCTYPE html>
				<html>
				<head>
					<base href="/subpath/">
					<meta name="generator" content="Hugo 0.80">
					<meta http-equiv="X-UA-Compatible" content="IE=7">
					<meta property="og:type" content="website">
					<meta charset="utf-8">
					<link rel="stylesheet" href="/style.css">
					<link rel="icon" href="/favicon.png">
					<script src="/app.js"></script>
				</head>
				<body>
					<!-- build version: v1.0.4 -->
				</body>
				</html>
			`,
			wantSignals: []fingerprint.Signal{
				{Source: "html:base", Value: "/subpath/"},
				{Source: "html:meta:name:generator", Value: "Hugo 0.80"},
				{Source: "html:meta:http-equiv:x-ua-compatible", Value: "IE=7"},
				{Source: "html:meta:property:og:type", Value: "website"},
				{Source: "html:meta:charset", Value: "utf-8"},
				{Source: "html:link:stylesheet", Value: "/style.css"},
				{Source: "html:link:icon", Value: "/favicon.png"},
				{Source: "html:script", Value: "/app.js"},
				{Source: "html:comment", Value: "build version: v1.0.4"},
			},
		},
		{
			name:        "Malformed HTML handles gracefully",
			contentType: "text/html",
			body: `
				<meta name="generator" content="Apache CMS" <invalid tag>
				<script src="/broken.js"
				<link rel="icon" href="/icon.png">
			`,
			wantSignals: []fingerprint.Signal{
				{Source: "html:meta:name:generator", Value: "Apache CMS"},
				{Source: "html:script", Value: "/broken.js"},
			},
		},
		{
			name:        "Unusual Casing and spaces handled",
			contentType: "text/html",
			body:        `  <mEtA   NaMe = "GeNeRaToR"   CoNtEnT = "WordPress" /> `,
			wantSignals: []fingerprint.Signal{
				{Source: "html:meta:name:generator", Value: "WordPress"},
			},
		},
		{
			name:        "Duplicate tags registered individually",
			contentType: "text/html",
			body: `
				<script src="/a.js"></script>
				<script src="/b.js"></script>
			`,
			wantSignals: []fingerprint.Signal{
				{Source: "html:script", Value: "/a.js"},
				{Source: "html:script", Value: "/b.js"},
			},
		},
		{
			name:        "Empty attributes ignored",
			contentType: "text/html",
			body: `
				<meta name="generator" content="">
				<script src="  "></script>
			`,
			wantSignals: nil,
		},
		{
			name:        "Comments keyword check and clamping",
			contentType: "text/html",
			body: `
				<!-- This is an uninteresting comment -->
				<!-- author: John Doe -->
				<!-- This generator comment is very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very long -->
			`,
			wantSignals: []fingerprint.Signal{
				{Source: "html:comment", Value: "author: John Doe"},
				{Source: "html:comment", Value: "This generator comment is very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very ve..."},
			},
		},
		{
			name:        "Framework-indicative attributes collected",
			contentType: "text/html",
			body: `
				<div id="app" class="test-class"></div>
				<span ng-version="12.0.1"></span>
				<div data-reactroot=""></div>
				<p data-v-34e892a=""></p>
				<main id="__next"></main>
			`,
			wantSignals: []fingerprint.Signal{
				{Source: "html:id", Value: "app"},
				{Source: "html:attr:ng-version", Value: "12.0.1"},
				{Source: "html:attr:data-reactroot", Value: ""},
				{Source: "html:attr:data-v-34e892a", Value: ""},
				{Source: "html:id", Value: "__next"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := fingerprint.NewCache()
			engine := fingerprint.NewEngine(cache)

			header := http.Header{}
			if tc.contentType != "" {
				header.Set("Content-Type", tc.contentType)
			}

			ctx := &fingerprint.Context{
				Host:   "example.com",
				Path:   tc.path,
				Header: header,
				Body:   []byte(tc.body),
			}

			engine.Analyze(ctx)

			fp := cache.Get("example.com")
			if fp == nil && len(tc.wantSignals) > 0 {
				t.Fatalf("Expected fingerprint to be created, got nil")
			}

			if fp == nil {
				return
			}

			rawSignals := fp.Signals()
			var gotSignals []fingerprint.Signal
			for _, s := range rawSignals {
				if strings.HasPrefix(s.Source, "html:") {
					gotSignals = append(gotSignals, s)
				}
			}

			if len(gotSignals) != len(tc.wantSignals) {
				t.Fatalf("Signals count mismatch: got %d, want %d.\nGot: %+v\nWant: %+v",
					len(gotSignals), len(tc.wantSignals), gotSignals, tc.wantSignals)
			}

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

func TestEngine_Headers_EdgeCases(t *testing.T) {
	cache := fingerprint.NewCache()
	engine := fingerprint.NewEngine(cache)

	// 1. Nil headers, nil body context
	ctxNil := &fingerprint.Context{
		Host:   "nil-ctx.com",
		Header: nil,
		Body:   nil,
	}
	engine.Analyze(ctxNil)
	if fp := cache.Get("nil-ctx.com"); fp == nil {
		t.Fatal("expected fingerprint to be created for nil context, got nil")
	}

	// 2. Extremely long header values
	longVal := strings.Repeat("a", 120000)
	ctxLong := &fingerprint.Context{
		Host:   "long-header.com",
		Header: http.Header{"Server": []string{longVal}},
	}
	engine.Analyze(ctxLong)
	fpLong := cache.Get("long-header.com")
	if fpLong == nil {
		t.Fatal("expected fingerprint for long header, got nil")
	}
	signals := fpLong.Signals()
	if len(signals) != 1 || signals[0].Value != longVal {
		t.Errorf("expected 1 signal with long value, got %+v", signals)
	}

	// 3. Duplicated headers and malformed cookies
	ctxDup := &fingerprint.Context{
		Host: "dup-headers.com",
		Header: http.Header{
			"Server": []string{"Apache", "nginx"},
			"Set-Cookie": []string{
				"=value", // empty name
				";",      // empty cookie
				"name=",  // empty value
				"a=b=c",  // multiple equals
				"  cookie_with_spaces  = val ",
			},
		},
	}
	engine.Analyze(ctxDup)
	fpDup := cache.Get("dup-headers.com")
	if fpDup == nil {
		t.Fatal("expected fingerprint for dup headers, got nil")
	}
	signalsDup := fpDup.Signals()
	// Should have 2 Server signals + valid cookies: "name" (from "name="), "a" (from "a=b=c"), "cookie_with_spaces" (from "cookie_with_spaces = val")
	// Total expected signals: 5
	var serverCount, cookieCount int
	for _, s := range signalsDup {
		if s.Source == "header:Server" {
			serverCount++
		}
		if s.Source == "cookie" {
			cookieCount++
			if s.Value != "name" && s.Value != "a" && s.Value != "cookie_with_spaces" {
				t.Errorf("unexpected cookie name parsed: %q", s.Value)
			}
		}
	}
	if serverCount != 2 {
		t.Errorf("expected 2 server signals, got %d", serverCount)
	}
	if cookieCount != 3 {
		t.Errorf("expected 3 valid cookie signals, got %d: %+v", cookieCount, signalsDup)
	}

	// 4. Set-Cookie header with only attributes and no name-value pair
	ctxAttrs := &fingerprint.Context{
		Host: "attrs-cookies.com",
		Header: http.Header{
			"Set-Cookie": []string{
				"Secure; HttpOnly",
				"SameSite=Lax",
				"HttpOnly",
			},
		},
	}
	engine.Analyze(ctxAttrs)
	fpAttrs := cache.Get("attrs-cookies.com")
	if fpAttrs == nil {
		t.Fatal("expected fingerprint for attributes cookies, got nil")
	}
	for _, s := range fpAttrs.Signals() {
		if s.Source == "cookie" {
			if s.Value == "Secure" || s.Value == "HttpOnly" {
				t.Errorf("Cookie parsing bug: attribute %q was parsed as a cookie name", s.Value)
			}
		}
	}
}

func TestEngine_HTML_EdgeCases(t *testing.T) {
	cache := fingerprint.NewCache()
	engine := fingerprint.NewEngine(cache)

	// 1. Deeply nested HTML structures (1500 nested tags to test stack limits)
	var sb strings.Builder
	for i := 0; i < 1500; i++ {
		sb.WriteString("<div>")
	}
	sb.WriteString("<meta name=\"generator\" content=\"NestedNestedNested\" />")
	for i := 0; i < 1500; i++ {
		sb.WriteString("</div>")
	}

	ctxDeep := &fingerprint.Context{
		Host: "deep-html.com",
		Header: http.Header{
			"Content-Type": []string{"text/html; charset=utf-8"},
		},
		Body: []byte(sb.String()),
	}
	engine.Analyze(ctxDeep)
	fpDeep := cache.Get("deep-html.com")
	if fpDeep == nil {
		t.Fatal("expected fingerprint for deep HTML, got nil")
	}
	signalsDeep := fpDeep.Signals()
	// Check that we successfully parsed the meta tag nested inside 1500 divs without stack overflow or crash
	var foundMeta bool
	for _, s := range signalsDeep {
		if s.Source == "html:meta:name:generator" && s.Value == "NestedNestedNested" {
			foundMeta = true
		}
	}
	if !foundMeta {
		t.Error("failed to find nested meta tag signal in deeply nested HTML structure")
	}

	// 2. HTML comments clamping and weird UTF-8 characters
	commentVal := "Theme: " + strings.Repeat("𠜎", 100)
	ctxComment := &fingerprint.Context{
		Host: "comment-html.com",
		Header: http.Header{
			"Content-Type": []string{"text/html"},
		},
		Body: []byte("<!-- " + commentVal + " -->"),
	}
	engine.Analyze(ctxComment)
	fpComment := cache.Get("comment-html.com")
	if fpComment == nil {
		t.Fatal("expected fingerprint for comment HTML, got nil")
	}
	var commentSignals []fingerprint.Signal
	for _, sig := range fpComment.Signals() {
		if sig.Source == "html:comment" {
			commentSignals = append(commentSignals, sig)
		}
	}
	if len(commentSignals) != 1 {
		t.Fatalf("expected 1 html:comment signal, got %d: %+v", len(commentSignals), commentSignals)
	}
	s := commentSignals[0]
	if !strings.HasSuffix(s.Value, "...") {
		t.Errorf("expected comment value to be clamped and end with '...', got: %q", s.Value)
	}
	if len(s.Value) > 256 {
		t.Errorf("expected comment value to be clamped to max 256 characters, got %d", len(s.Value))
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

func BenchmarkHTMLDetector_Standard(b *testing.B) {
	cache := fingerprint.NewCache()
	engine := fingerprint.NewEngine(cache)

	header := http.Header{}
	header.Set("Content-Type", "text/html; charset=utf-8")

	// 20KB typical HTML boilerplate containing meta tags, script/link tags, and attributes
	var buf bytes.Buffer
	buf.WriteString(`<!DOCTYPE html><html><head><title>Benchmark Page</title>`)
	for i := 0; i < 20; i++ {
		buf.WriteString(fmt.Sprintf(`<meta name="generator" content="Hugo %d.0">`, i))
		buf.WriteString(fmt.Sprintf(`<meta property="og:title" content="Meta Title %d">`, i))
		buf.WriteString(fmt.Sprintf(`<link rel="stylesheet" href="/style-%d.css">`, i))
		buf.WriteString(fmt.Sprintf(`<script src="/js/script-%d.js"></script>`, i))
	}
	buf.WriteString(`</head><body><div id="app" ng-version="12.0">`)
	buf.WriteString(`<!-- build version: v1.2.3 -->`)
	for i := 0; i < 50; i++ {
		buf.WriteString(fmt.Sprintf(`<div class="item" data-v-34e8%d="">Element %d</div>`, i, i))
	}
	buf.WriteString(`</div></body></html>`)

	ctx := &fingerprint.Context{
		Host:   "bench.com",
		Header: header,
		Body:   buf.Bytes(),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		engine.Analyze(ctx)
	}
}
