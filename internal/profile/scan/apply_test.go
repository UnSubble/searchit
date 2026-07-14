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
			name: "all fields present with duration strings",
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
	cfg := config.Default()

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
	normalizePathsVal := true
	collapseSlashesVal := true
	excludeStatusVal := "500"
	recurseOnVal := "200,302"
	includeSizeVal := "500-1000"
	excludeSizeVal := "123"
	includeHeadersVal := []string{"Server=nginx"}
	excludeHeadersVal := []string{"X-Header=val"}

	o := scan.Overlay{
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
		NormalizePaths:  &normalizePathsVal,
		CollapseSlashes: &collapseSlashesVal,
		ExcludeStatus:   &excludeStatusVal,
		RecurseOn:       &recurseOnVal,
		IncludeSize:     &includeSizeVal,
		ExcludeSize:     &excludeSizeVal,
		IncludeHeaders:  &includeHeadersVal,
		ExcludeHeaders:  &excludeHeadersVal,
	}

	scan.Apply(&cfg, o)

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

	// Verify strategy parsing ignore on invalid value
	invalidStrategy := "invalid"
	scan.Apply(&cfg, scan.Overlay{Strategy: &invalidStrategy})
	if cfg.Strategy != recursion.DFS {
		t.Errorf("invalid strategy should not change strategy, got %v", cfg.Strategy)
	}
}
