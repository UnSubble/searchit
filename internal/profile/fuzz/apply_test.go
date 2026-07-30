package fuzz_test

import (
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/profile/fuzz"
	"gopkg.in/yaml.v3"
)

func TestFuzzUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		yamlData string
		verify   func(t *testing.T, o fuzz.Overlay)
		wantErr  bool
	}{
		{
			name: "all legacy fields with duration strings",
			yamlData: `
threads: 16
timeout: 30s
connect-timeout: 5s
strategy: bfs
delay: 100ms
rate: 2.5
format: json
quiet: true
follow-redirects: true
exclude-status: "404,500"
include-size: "100-200"
exclude-size: "0"
include-headers: ["X-Token=abc"]
exclude-headers: ["Server=bad"]
method: POST
data: "body=FUZZ"
headers: ["Content-Type=application/json"]
cookies: "session=abc"
request: "req.txt"
random-agent: true
`,
			verify: func(t *testing.T, o fuzz.Overlay) {
				if o.Threads == nil || *o.Threads != 16 {
					t.Errorf("threads = %v, want 16", o.Threads)
				}
				if o.Timeout == nil || *o.Timeout != 30*time.Second {
					t.Errorf("timeout = %v, want 30s", o.Timeout)
				}
				if o.ConnectTimeout == nil || *o.ConnectTimeout != 5*time.Second {
					t.Errorf("connect-timeout = %v, want 5s", o.ConnectTimeout)
				}
				if o.Strategy == nil || *o.Strategy != "bfs" {
					t.Errorf("strategy = %v, want bfs", o.Strategy)
				}
				if o.Delay == nil || *o.Delay != 100*time.Millisecond {
					t.Errorf("delay = %v, want 100ms", o.Delay)
				}
				if o.Rate == nil || *o.Rate != 2.5 {
					t.Errorf("rate = %v, want 2.5", o.Rate)
				}
				if o.Format == nil || *o.Format != "json" {
					t.Errorf("format = %v, want json", o.Format)
				}
				if o.Quiet == nil || !*o.Quiet {
					t.Errorf("quiet = %v, want true", o.Quiet)
				}
				if o.FollowRedirects == nil || !*o.FollowRedirects {
					t.Errorf("follow-redirects = %v, want true", o.FollowRedirects)
				}
				if o.ExcludeStatus == nil || *o.ExcludeStatus != "404,500" {
					t.Errorf("exclude-status = %v, want 404,500", o.ExcludeStatus)
				}
				if o.IncludeSize == nil || *o.IncludeSize != "100-200" {
					t.Errorf("include-size = %v, want 100-200", o.IncludeSize)
				}
				if o.ExcludeSize == nil || *o.ExcludeSize != "0" {
					t.Errorf("exclude-size = %v, want 0", o.ExcludeSize)
				}
				if o.IncludeHeaders == nil || len(*o.IncludeHeaders) != 1 {
					t.Errorf("include-headers = %v", o.IncludeHeaders)
				}
				if o.ExcludeHeaders == nil || len(*o.ExcludeHeaders) != 1 {
					t.Errorf("exclude-headers = %v", o.ExcludeHeaders)
				}
				if o.Method == nil || *o.Method != "POST" {
					t.Errorf("method = %v, want POST", o.Method)
				}
				if o.Data == nil || *o.Data != "body=FUZZ" {
					t.Errorf("data = %v, want body=FUZZ", o.Data)
				}
				if o.Cookies == nil || *o.Cookies != "session=abc" {
					t.Errorf("cookies = %v", o.Cookies)
				}
				if o.RandomAgent == nil || !*o.RandomAgent {
					t.Errorf("random-agent = %v, want true", o.RandomAgent)
				}
			},
		},
		{
			name: "new fields: url, ext, match-status, match-regex, proxy, log-count",
			yamlData: `
url: "http://example.com/FUZZ"
ext: [".php"]
match-status: "200"
match-regex: ["admin"]
filter-regex: ["error"]
match-content: ["text/html"]
filter-content: ["image/"]
proxy: "http://127.0.0.1:8080"
max-redirects: 3
show-headers: true
show-title: true
adaptive: true
user-agent: "FuzzBot/1.0"
log-count: 15
`,
			verify: func(t *testing.T, o fuzz.Overlay) {
				if o.URL == nil || *o.URL != "http://example.com/FUZZ" {
					t.Errorf("url = %v", o.URL)
				}
				if o.Extensions == nil || len(*o.Extensions) != 1 {
					t.Errorf("ext = %v", o.Extensions)
				}
				if o.MatchStatus == nil || *o.MatchStatus != "200" {
					t.Errorf("match-status = %v", o.MatchStatus)
				}
				if o.MatchRegex == nil || len(*o.MatchRegex) != 1 {
					t.Errorf("match-regex = %v", o.MatchRegex)
				}
				if o.FilterRegex == nil || len(*o.FilterRegex) != 1 {
					t.Errorf("filter-regex = %v", o.FilterRegex)
				}
				if o.MatchContent == nil || len(*o.MatchContent) != 1 {
					t.Errorf("match-content = %v", o.MatchContent)
				}
				if o.FilterContent == nil || len(*o.FilterContent) != 1 {
					t.Errorf("filter-content = %v", o.FilterContent)
				}
				if o.Proxy == nil || *o.Proxy != "http://127.0.0.1:8080" {
					t.Errorf("proxy = %v", o.Proxy)
				}
				if o.MaxRedirects == nil || *o.MaxRedirects != 3 {
					t.Errorf("max-redirects = %v", o.MaxRedirects)
				}
				if o.ShowHeaders == nil || !*o.ShowHeaders {
					t.Errorf("show-headers = %v", o.ShowHeaders)
				}
				if o.ShowTitle == nil || !*o.ShowTitle {
					t.Errorf("show-title = %v", o.ShowTitle)
				}
				if o.Adaptive == nil || !*o.Adaptive {
					t.Errorf("adaptive = %v", o.Adaptive)
				}
				if o.UserAgent == nil || *o.UserAgent != "FuzzBot/1.0" {
					t.Errorf("user-agent = %v", o.UserAgent)
				}
			},
		},
		{
			// Bug fix: fuzz-extra/cookie.yaml uses "cookie:" (singular) which was
			// previously silently ignored because fuzz.Overlay only had "cookies:".
			// Verify the alias is now decoded correctly.
			name:     "cookie alias (singular) decoded as cookies",
			yamlData: `cookie: "FOO=BAR"`,
			verify: func(t *testing.T, o fuzz.Overlay) {
				if o.Cookies == nil || *o.Cookies != "FOO=BAR" {
					t.Errorf("cookie alias: Cookies = %v, want FOO=BAR", o.Cookies)
				}
			},
		},
		{
			name:     "cookies plural takes precedence over cookie singular",
			yamlData: "cookies: \"session=abc\"\ncookie: \"other=xyz\"",
			verify: func(t *testing.T, o fuzz.Overlay) {
				if o.Cookies == nil || *o.Cookies != "session=abc" {
					t.Errorf("cookies plural should win: Cookies = %v, want session=abc", o.Cookies)
				}
			},
		},
		{
			name: "singular header aliases",
			yamlData: `
include-header: ["X-Foo=bar"]
exclude-header: ["X-Baz=qux"]
`,
			verify: func(t *testing.T, o fuzz.Overlay) {
				if o.IncludeHeaders == nil || len(*o.IncludeHeaders) != 1 {
					t.Errorf("include-header alias: %v", o.IncludeHeaders)
				}
				if o.ExcludeHeaders == nil || len(*o.ExcludeHeaders) != 1 {
					t.Errorf("exclude-header alias: %v", o.ExcludeHeaders)
				}
			},
		},
		{
			name:     "empty overlay",
			yamlData: `{}`,
			verify: func(t *testing.T, o fuzz.Overlay) {
				if o.Threads != nil {
					t.Errorf("expected nil threads, got %v", o.Threads)
				}
				if o.URL != nil {
					t.Errorf("expected nil url, got %v", o.URL)
				}
			},
		},
		{
			name:     "invalid timeout string",
			yamlData: `timeout: bad`,
			wantErr:  true,
		},
		{
			name:     "invalid delay format",
			yamlData: `delay: [1,2]`,
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var o fuzz.Overlay
			err := yaml.Unmarshal([]byte(tc.yamlData), &o)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unmarshal error = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && tc.verify != nil {
				tc.verify(t, o)
			}
		})
	}
}

func TestFuzzApply(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() fuzz.Overlay
		verify func(t *testing.T, cfg config.Config)
	}{
		{
			name: "url sets cfg.URLs",
			setup: func() fuzz.Overlay {
				u := "http://example.com/FUZZ"
				return fuzz.Overlay{URL: &u}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if len(cfg.URLs) != 1 || cfg.URLs[0] != "http://example.com/FUZZ" {
					t.Errorf("URLs = %v, want [http://example.com/FUZZ]", cfg.URLs)
				}
			},
		},
		{
			name: "wordlist sets cfg.Wordlist",
			setup: func() fuzz.Overlay {
				w := "custom.txt"
				return fuzz.Overlay{Wordlist: &w}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if cfg.Wordlist != "custom.txt" {
					t.Errorf("Wordlist = %q", cfg.Wordlist)
				}
			},
		},
		{
			name: "connect-timeout is applied (was pre-existing bug)",
			setup: func() fuzz.Overlay {
				ct := 7 * time.Second
				return fuzz.Overlay{ConnectTimeout: &ct}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if cfg.ConnectTimeout != 7*time.Second {
					t.Errorf("ConnectTimeout = %v, want 7s", cfg.ConnectTimeout)
				}
			},
		},
		{
			name: "strategy sets cfg.FuzzStrategy",
			setup: func() fuzz.Overlay {
				s := "dfs"
				return fuzz.Overlay{Strategy: &s}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if cfg.FuzzStrategy != "dfs" {
					t.Errorf("FuzzStrategy = %q, want dfs", cfg.FuzzStrategy)
				}
			},
		},
		{
			name: "match-status sets cfg.Status.Include",
			setup: func() fuzz.Overlay {
				ms := "200,301"
				return fuzz.Overlay{MatchStatus: &ms}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if !cfg.Status.Include.Match(200) {
					t.Errorf("Status.Include should match 200")
				}
			},
		},
		{
			name: "match-regex sets cfg.MatchRegex",
			setup: func() fuzz.Overlay {
				mr := []string{"admin", "secret"}
				return fuzz.Overlay{MatchRegex: &mr}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if len(cfg.MatchRegex) != 2 {
					t.Errorf("MatchRegex = %v, want 2 items", cfg.MatchRegex)
				}
			},
		},
		{
			name: "filter-regex sets cfg.FilterRegex",
			setup: func() fuzz.Overlay {
				fr := []string{"error"}
				return fuzz.Overlay{FilterRegex: &fr}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if len(cfg.FilterRegex) != 1 || cfg.FilterRegex[0] != "error" {
					t.Errorf("FilterRegex = %v", cfg.FilterRegex)
				}
			},
		},
		{
			name: "match-content and filter-content",
			setup: func() fuzz.Overlay {
				mc := []string{"application/json"}
				fc := []string{"text/plain"}
				return fuzz.Overlay{MatchContent: &mc, FilterContent: &fc}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if len(cfg.MatchContent) != 1 {
					t.Errorf("MatchContent = %v", cfg.MatchContent)
				}
				if len(cfg.FilterContent) != 1 {
					t.Errorf("FilterContent = %v", cfg.FilterContent)
				}
			},
		},
		{
			name: "max-redirects",
			setup: func() fuzz.Overlay {
				mr := 3
				return fuzz.Overlay{MaxRedirects: &mr}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if cfg.MaxRedirects != 3 {
					t.Errorf("MaxRedirects = %d, want 3", cfg.MaxRedirects)
				}
			},
		},
		{
			name: "proxy",
			setup: func() fuzz.Overlay {
				p := "http://127.0.0.1:8080"
				return fuzz.Overlay{Proxy: &p}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if cfg.Proxy != "http://127.0.0.1:8080" {
					t.Errorf("Proxy = %q", cfg.Proxy)
				}
			},
		},
		{
			name: "show-headers and show-title",
			setup: func() fuzz.Overlay {
				sh := true
				st := true
				return fuzz.Overlay{ShowHeaders: &sh, ShowTitle: &st}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if !cfg.ShowHeaders {
					t.Errorf("ShowHeaders = false")
				}
				if !cfg.ShowTitle {
					t.Errorf("ShowTitle = false")
				}
			},
		},
		{
			name: "adaptive",
			setup: func() fuzz.Overlay {
				a := true
				return fuzz.Overlay{Adaptive: &a}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if !cfg.Adaptive {
					t.Errorf("Adaptive = false")
				}
			},
		},
		{
			name: "user-agent",
			setup: func() fuzz.Overlay {
				ua := "FuzzBot/1.0"
				return fuzz.Overlay{UserAgent: &ua}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if cfg.UserAgent != "FuzzBot/1.0" {
					t.Errorf("UserAgent = %q", cfg.UserAgent)
				}
			},
		},

		{
			name: "cookie alias correctly sets cfg.Cookies",
			setup: func() fuzz.Overlay {
				// Simulate profile that used "cookie:" (singular, Bug 1 fix).
				var o fuzz.Overlay
				_ = yaml.Unmarshal([]byte(`cookie: "FOO=BAR"`), &o)
				return o
			},
			verify: func(t *testing.T, cfg config.Config) {
				if cfg.Cookies != "FOO=BAR" {
					t.Errorf("Cookies = %q, want FOO=BAR", cfg.Cookies)
				}
			},
		},
		{
			name: "invalid match-regex dropped silently in apply",
			setup: func() fuzz.Overlay {
				mr := []string{"valid.*", "[bad"}
				return fuzz.Overlay{MatchRegex: &mr}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if len(cfg.MatchRegex) != 1 || cfg.MatchRegex[0] != "valid.*" {
					t.Errorf("MatchRegex = %v, want [valid.*]", cfg.MatchRegex)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			o := tc.setup()
			fuzz.Apply(&cfg, o)
			tc.verify(t, cfg)
		})
	}
}
