package robots_test

import (
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/robots"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantRules []robots.Directive
	}{
		{
			name:      "Empty input",
			input:     "",
			wantRules: nil,
		},
		{
			name: "Standard Allow and Disallow directives",
			input: `
				User-agent: *
				Disallow: /admin/
				Allow: /public/assets/
				Disallow: /tmp
			`,
			wantRules: []robots.Directive{
				{Type: robots.Disallow, Path: "/admin/"},
				{Type: robots.Allow, Path: "/public/assets/"},
				{Type: robots.Disallow, Path: "/tmp"},
			},
		},
		{
			name: "Comment lines and inline comments ignored",
			input: `
				# This is a comment
				Disallow: /private # inline comment
				# Another comment
				Allow: /images/logo.png
			`,
			wantRules: []robots.Directive{
				{Type: robots.Disallow, Path: "/private"},
				{Type: robots.Allow, Path: "/images/logo.png"},
			},
		},
		{
			name: "Whitespace variations handled",
			input: `
				disallow:    /trimmed-space   
				allow:	/tab-separated
			`,
			wantRules: []robots.Directive{
				{Type: robots.Disallow, Path: "/trimmed-space"},
				{Type: robots.Allow, Path: "/tab-separated"},
			},
		},
		{
			name: "Malformed lines handled gracefully",
			input: `
				random gibberish text
				Disallow: /valid
				Allow
				: /no-key
			`,
			wantRules: []robots.Directive{
				{Type: robots.Disallow, Path: "/valid"},
			},
		},
		{
			name: "Unsupported directives ignored",
			input: `
				Sitemap: http://example.com/sitemap.xml
				Crawl-delay: 10
				Disallow: /admin
			`,
			wantRules: []robots.Directive{
				{Type: robots.Disallow, Path: "/admin"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := robots.Parse(strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			if len(got) != len(tc.wantRules) {
				t.Fatalf("Parse() count mismatch: got %d, want %d.\nGot: %+v\nWant: %+v",
					len(got), len(tc.wantRules), got, tc.wantRules)
			}

			for i, r := range got {
				if r.Type != tc.wantRules[i].Type || r.Path != tc.wantRules[i].Path {
					t.Errorf("got[%d] = %+v, want %+v", i, r, tc.wantRules[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkParse_Standard(b *testing.B) {
	input := `
		# robots.txt benchmark file
		User-agent: *
		Disallow: /admin/
		Disallow: /tmp/
		Disallow: /private/
		Allow: /public/
		Allow: /assets/
		Crawl-delay: 5
		Sitemap: http://example.com/sitemap.xml
	`
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = robots.Parse(strings.NewReader(input))
	}
}
