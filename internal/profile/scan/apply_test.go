package scan_test

import (
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/profile/scan"
	"github.com/unsubble/searchit/internal/recursion"
	"gopkg.in/yaml.v3"
)

func TestUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		yamlData string
		verify   func(t *testing.T, o scan.Overlay)
		wantErr  bool
	}{
		{
			name: "all legacy fields present with duration strings",
			yamlData: `
threads: 10
timeout: 5s
connect-timeout: 3s
recursive: true
max-depth: 4
strategy: dfs
delay: 250ms
rate: 5.5
output: json
quiet: true
normalize-paths: true
collapse-slashes: true
exclude-status: "404,500"
recurse-on: "200-300"
include-size: "100-200"
exclude-size: "0"
include-headers: ["Server=nginx"]
exclude-headers: ["Server=Apache"]
`,
			verify: func(t *testing.T, o scan.Overlay) {
				if o.Threads == nil || *o.Threads != 10 {
					t.Errorf("threads = %v, want 10", o.Threads)
				}
				if o.Timeout == nil || *o.Timeout != 5*time.Second {
					t.Errorf("timeout = %v, want 5s", o.Timeout)
				}
				if o.ConnectTimeout == nil || *o.ConnectTimeout != 3*time.Second {
					t.Errorf("connect-timeout = %v, want 3s", o.ConnectTimeout)
				}
				if o.Recursive == nil || !*o.Recursive {
					t.Errorf("recursive = %v, want true", o.Recursive)
				}
				if o.MaxDepth == nil || *o.MaxDepth != 4 {
					t.Errorf("max-depth = %v, want 4", o.MaxDepth)
				}
				if o.Strategy == nil || *o.Strategy != "dfs" {
					t.Errorf("strategy = %v, want dfs", o.Strategy)
				}
				if o.Delay == nil || *o.Delay != 250*time.Millisecond {
					t.Errorf("delay = %v, want 250ms", o.Delay)
				}
				if o.Rate == nil || *o.Rate != 5.5 {
					t.Errorf("rate = %v, want 5.5", o.Rate)
				}
				if o.Format == nil || *o.Format != "json" {
					t.Errorf("format = %v, want json", o.Format)
				}
				if o.Quiet == nil || !*o.Quiet {
					t.Errorf("quiet = %v, want true", o.Quiet)
				}
				if o.NormalizePaths == nil || !*o.NormalizePaths {
					t.Errorf("normalize-paths = %v, want true", o.NormalizePaths)
				}
				if o.CollapseSlashes == nil || !*o.CollapseSlashes {
					t.Errorf("collapse-slashes = %v, want true", o.CollapseSlashes)
				}
				if o.ExcludeStatus == nil || *o.ExcludeStatus != "404,500" {
					t.Errorf("exclude-status = %v, want 404,500", o.ExcludeStatus)
				}
				if o.RecurseOn == nil || *o.RecurseOn != "200-300" {
					t.Errorf("recurse-on = %v, want 200-300", o.RecurseOn)
				}
				if o.IncludeSize == nil || *o.IncludeSize != "100-200" {
					t.Errorf("include-size = %v, want 100-200", o.IncludeSize)
				}
				if o.ExcludeSize == nil || *o.ExcludeSize != "0" {
					t.Errorf("exclude-size = %v, want 0", o.ExcludeSize)
				}
				if o.IncludeHeaders == nil || len(*o.IncludeHeaders) != 1 || (*o.IncludeHeaders)[0] != "Server=nginx" {
					t.Errorf("include-headers = %v", o.IncludeHeaders)
				}
				if o.ExcludeHeaders == nil || len(*o.ExcludeHeaders) != 1 || (*o.ExcludeHeaders)[0] != "Server=Apache" {
					t.Errorf("exclude-headers = %v", o.ExcludeHeaders)
				}
			},
		},
		{
			name: "new fields: url, url-file, ext, match-status, match-regex, proxy",
			yamlData: `
url: "http://example.com"
url-file: "/tmp/urls.txt"
ext: [".php", ".asp"]
match-status: "200,301"
match-regex: ["admin", "secret"]
filter-regex: ["error"]
match-content: ["application/json"]
filter-content: ["text/plain"]
proxy: "http://127.0.0.1:8080"
max-redirects: 5
show-headers: true
show-title: true
adaptive: true
user-agent: "Mozilla/5.0"
log-count: 20
`,
			verify: func(t *testing.T, o scan.Overlay) {
				if o.URL == nil || *o.URL != "http://example.com" {
					t.Errorf("url = %v, want http://example.com", o.URL)
				}
				if o.URLFile == nil || *o.URLFile != "/tmp/urls.txt" {
					t.Errorf("url-file = %v, want /tmp/urls.txt", o.URLFile)
				}
				if o.Extensions == nil || len(*o.Extensions) != 2 {
					t.Errorf("ext = %v, want 2 items", o.Extensions)
				}
				if o.MatchStatus == nil || *o.MatchStatus != "200,301" {
					t.Errorf("match-status = %v, want 200,301", o.MatchStatus)
				}
				if o.MatchRegex == nil || len(*o.MatchRegex) != 2 {
					t.Errorf("match-regex = %v, want 2 items", o.MatchRegex)
				}
				if o.FilterRegex == nil || len(*o.FilterRegex) != 1 {
					t.Errorf("filter-regex = %v, want 1 item", o.FilterRegex)
				}
				if o.MatchContent == nil || len(*o.MatchContent) != 1 {
					t.Errorf("match-content = %v, want 1 item", o.MatchContent)
				}
				if o.FilterContent == nil || len(*o.FilterContent) != 1 {
					t.Errorf("filter-content = %v, want 1 item", o.FilterContent)
				}
				if o.Proxy == nil || *o.Proxy != "http://127.0.0.1:8080" {
					t.Errorf("proxy = %v, want http://127.0.0.1:8080", o.Proxy)
				}
				if o.MaxRedirects == nil || *o.MaxRedirects != 5 {
					t.Errorf("max-redirects = %v, want 5", o.MaxRedirects)
				}
				if o.ShowHeaders == nil || !*o.ShowHeaders {
					t.Errorf("show-headers = %v, want true", o.ShowHeaders)
				}
				if o.ShowTitle == nil || !*o.ShowTitle {
					t.Errorf("show-title = %v, want true", o.ShowTitle)
				}
				if o.Adaptive == nil || !*o.Adaptive {
					t.Errorf("adaptive = %v, want true", o.Adaptive)
				}
				if o.UserAgent == nil || *o.UserAgent != "Mozilla/5.0" {
					t.Errorf("user-agent = %v, want Mozilla/5.0", o.UserAgent)
				}
			},
		},
		{
			name: "singular header key aliases",
			yamlData: `
include-header: ["Server=nginx"]
exclude-header: ["Server=Apache"]
`,
			verify: func(t *testing.T, o scan.Overlay) {
				if o.IncludeHeaders == nil || len(*o.IncludeHeaders) != 1 || (*o.IncludeHeaders)[0] != "Server=nginx" {
					t.Errorf("include-header = %v", o.IncludeHeaders)
				}
				if o.ExcludeHeaders == nil || len(*o.ExcludeHeaders) != 1 || (*o.ExcludeHeaders)[0] != "Server=Apache" {
					t.Errorf("exclude-header = %v", o.ExcludeHeaders)
				}
			},
		},
		{
			name: "durations as integers",
			yamlData: `
timeout: 15
connect-timeout: 4
delay: 500
`,
			verify: func(t *testing.T, o scan.Overlay) {
				if o.Timeout == nil || *o.Timeout != 15*time.Second {
					t.Errorf("timeout = %v, want 15s", o.Timeout)
				}
				if o.ConnectTimeout == nil || *o.ConnectTimeout != 4*time.Second {
					t.Errorf("connect-timeout = %v, want 4s", o.ConnectTimeout)
				}
				if o.Delay == nil || *o.Delay != 500*time.Millisecond {
					t.Errorf("delay = %v, want 500ms", o.Delay)
				}
			},
		},
		{
			name:     "empty overlay",
			yamlData: `{}`,
			verify: func(t *testing.T, o scan.Overlay) {
				if o.Threads != nil {
					t.Errorf("expected nil threads, got %v", o.Threads)
				}
				if o.Timeout != nil {
					t.Errorf("expected nil timeout, got %v", o.Timeout)
				}
				if o.URL != nil {
					t.Errorf("expected nil url, got %v", o.URL)
				}
			},
		},
		{
			name:     "invalid timeout string",
			yamlData: `timeout: invalid`,
			wantErr:  true,
		},
		{
			name:     "invalid timeout array",
			yamlData: `timeout: [1, 2]`,
			wantErr:  true,
		},
		{
			name:     "invalid connect-timeout string",
			yamlData: `connect-timeout: invalid`,
			wantErr:  true,
		},
		{
			name:     "invalid connect-timeout array",
			yamlData: `connect-timeout: [1, 2]`,
			wantErr:  true,
		},
		{
			name:     "invalid delay string",
			yamlData: `delay: invalid`,
			wantErr:  true,
		},
		{
			name:     "invalid delay array",
			yamlData: `delay: [1, 2]`,
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var o scan.Overlay
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

func TestApply(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() scan.Overlay
		verify func(t *testing.T, cfg config.Config)
	}{
		{
			name: "legacy fields",
			setup: func() scan.Overlay {
				threadsVal := 64
				recursiveVal := true
				wordlistVal := "custom.txt"
				timeoutVal := 20 * time.Second
				connectTimeoutVal := 5 * time.Second
				maxDepthVal := uint16(5)
				strategyVal := "dfs"
				delayVal := 100 * time.Millisecond
				rateVal := 12.5
				outputVal := "json"
				quietVal := true
				followRedirectsVal := true
				normalizePathsVal := true
				collapseSlashesVal := true
				excludeStatusVal := "500"
				recurseOnVal := "200,302"
				includeSizeVal := "500-1000"
				excludeSizeVal := "123"
				includeHeadersVal := []string{"Server=nginx"}
				excludeHeadersVal := []string{"X-Header=val"}
				return scan.Overlay{
					Wordlist:        &wordlistVal,
					Threads:         &threadsVal,
					Timeout:         &timeoutVal,
					ConnectTimeout:  &connectTimeoutVal,
					Recursive:       &recursiveVal,
					MaxDepth:        &maxDepthVal,
					Strategy:        &strategyVal,
					Delay:           &delayVal,
					Rate:            &rateVal,
					Format:          &outputVal,
					Quiet:           &quietVal,
					FollowRedirects: &followRedirectsVal,
					NormalizePaths:  &normalizePathsVal,
					CollapseSlashes: &collapseSlashesVal,
					ExcludeStatus:   &excludeStatusVal,
					RecurseOn:       &recurseOnVal,
					IncludeSize:     &includeSizeVal,
					ExcludeSize:     &excludeSizeVal,
					IncludeHeaders:  &includeHeadersVal,
					ExcludeHeaders:  &excludeHeadersVal,
				}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if cfg.Wordlist != "custom.txt" {
					t.Errorf("Wordlist = %s", cfg.Wordlist)
				}
				if cfg.Threads != 64 {
					t.Errorf("Threads = %d", cfg.Threads)
				}
				if cfg.Timeout != 20*time.Second {
					t.Errorf("Timeout = %v", cfg.Timeout)
				}
				if cfg.ConnectTimeout != 5*time.Second {
					t.Errorf("ConnectTimeout = %v", cfg.ConnectTimeout)
				}
				if !cfg.Recursive {
					t.Errorf("Recursive = false")
				}
				if cfg.MaxDepth != 5 {
					t.Errorf("MaxDepth = %d", cfg.MaxDepth)
				}
				if cfg.Strategy != recursion.DFS {
					t.Errorf("Strategy = %v", cfg.Strategy)
				}
				if cfg.Delay != 100*time.Millisecond {
					t.Errorf("Delay = %v", cfg.Delay)
				}
				if cfg.Rate != 12.5 {
					t.Errorf("Rate = %f", cfg.Rate)
				}
				if cfg.OutputFormat != "json" {
					t.Errorf("OutputFormat = %s", cfg.OutputFormat)
				}
				if !cfg.Quiet {
					t.Errorf("Quiet = false")
				}
				if !cfg.FollowRedirects {
					t.Errorf("FollowRedirects = false")
				}
				if !cfg.Paths.NormalizePaths {
					t.Errorf("NormalizePaths = false")
				}
				if !cfg.Paths.CollapseSlashes {
					t.Errorf("CollapseSlashes = false")
				}
				if !cfg.Status.Exclude.Match(500) {
					t.Errorf("ExcludeStatus failed")
				}
				if !cfg.RecurseOn.Match(302) {
					t.Errorf("RecurseOn failed")
				}
				if !cfg.IncludeSize.Match(600) {
					t.Errorf("IncludeSize failed")
				}
				if !cfg.ExcludeSize.Match(123) {
					t.Errorf("ExcludeSize failed")
				}
				if len(cfg.IncludeHeaders) != 1 || cfg.IncludeHeaders[0].Name != "Server" {
					t.Errorf("IncludeHeaders failed")
				}
				if len(cfg.ExcludeHeaders) != 1 || cfg.ExcludeHeaders[0].Value != "val" {
					t.Errorf("ExcludeHeaders failed")
				}
			},
		},
		{
			name: "url sets cfg.URLs",
			setup: func() scan.Overlay {
				u := "http://example.com"
				return scan.Overlay{URL: &u}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if len(cfg.URLs) != 1 || cfg.URLs[0] != "http://example.com" {
					t.Errorf("URLs = %v, want [http://example.com]", cfg.URLs)
				}
			},
		},
		{
			name: "url-file sets cfg.URLFile",
			setup: func() scan.Overlay {
				f := "/tmp/targets.txt"
				return scan.Overlay{URLFile: &f}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if cfg.URLFile != "/tmp/targets.txt" {
					t.Errorf("URLFile = %q, want /tmp/targets.txt", cfg.URLFile)
				}
			},
		},
		{
			name: "ext sets cfg.Extensions",
			setup: func() scan.Overlay {
				exts := []string{".php", ".asp"}
				return scan.Overlay{Extensions: &exts}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if len(cfg.Extensions) == 0 {
					t.Errorf("Extensions empty, want .php .asp")
				}
			},
		},
		{
			name: "match-status sets cfg.Status.Include",
			setup: func() scan.Overlay {
				ms := "200,201"
				return scan.Overlay{MatchStatus: &ms}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if !cfg.Status.Include.Match(200) {
					t.Errorf("Status.Include should match 200")
				}
			},
		},
		{
			name: "match-regex sets cfg.MatchRegex",
			setup: func() scan.Overlay {
				mr := []string{"admin", "secret"}
				return scan.Overlay{MatchRegex: &mr}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if len(cfg.MatchRegex) != 2 {
					t.Errorf("MatchRegex = %v, want 2 items", cfg.MatchRegex)
				}
			},
		},
		{
			name: "filter-regex sets cfg.FilterRegex",
			setup: func() scan.Overlay {
				fr := []string{"error", "404"}
				return scan.Overlay{FilterRegex: &fr}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if len(cfg.FilterRegex) != 2 {
					t.Errorf("FilterRegex = %v, want 2 items", cfg.FilterRegex)
				}
			},
		},
		{
			name: "match-content and filter-content",
			setup: func() scan.Overlay {
				mc := []string{"application/json"}
				fc := []string{"text/plain"}
				return scan.Overlay{MatchContent: &mc, FilterContent: &fc}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if len(cfg.MatchContent) != 1 || cfg.MatchContent[0] != "application/json" {
					t.Errorf("MatchContent = %v", cfg.MatchContent)
				}
				if len(cfg.FilterContent) != 1 || cfg.FilterContent[0] != "text/plain" {
					t.Errorf("FilterContent = %v", cfg.FilterContent)
				}
			},
		},
		{
			name: "max-redirects sets cfg.MaxRedirects",
			setup: func() scan.Overlay {
				mr := 5
				return scan.Overlay{MaxRedirects: &mr}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if cfg.MaxRedirects != 5 {
					t.Errorf("MaxRedirects = %d, want 5", cfg.MaxRedirects)
				}
			},
		},
		{
			name: "proxy sets cfg.Proxy",
			setup: func() scan.Overlay {
				p := "http://127.0.0.1:8080"
				return scan.Overlay{Proxy: &p}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if cfg.Proxy != "http://127.0.0.1:8080" {
					t.Errorf("Proxy = %q", cfg.Proxy)
				}
			},
		},
		{
			name: "show-headers and show-title",
			setup: func() scan.Overlay {
				sh := true
				st := true
				return scan.Overlay{ShowHeaders: &sh, ShowTitle: &st}
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
			name: "adaptive sets cfg.Adaptive",
			setup: func() scan.Overlay {
				a := true
				return scan.Overlay{Adaptive: &a}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if !cfg.Adaptive {
					t.Errorf("Adaptive = false")
				}
			},
		},
		{
			name: "user-agent sets cfg.UserAgent",
			setup: func() scan.Overlay {
				ua := "MyBot/1.0"
				return scan.Overlay{UserAgent: &ua}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if cfg.UserAgent != "MyBot/1.0" {
					t.Errorf("UserAgent = %q", cfg.UserAgent)
				}
			},
		},

		{
			name: "invalid strategy does not change strategy",
			setup: func() scan.Overlay {
				bad := "invalid"
				return scan.Overlay{Strategy: &bad}
			},
			verify: func(t *testing.T, cfg config.Config) {
				if cfg.Strategy != recursion.BFS {
					t.Errorf("invalid strategy should not change strategy, got %v", cfg.Strategy)
				}
			},
		},
		{
			name: "invalid match-regex entries are silently dropped",
			setup: func() scan.Overlay {
				mr := []string{"valid.*", "[invalid"}
				return scan.Overlay{MatchRegex: &mr}
			},
			verify: func(t *testing.T, cfg config.Config) {
				// Only the valid pattern should survive; the invalid one is dropped.
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
			scan.Apply(&cfg, o)
			tc.verify(t, cfg)
		})
	}
}
