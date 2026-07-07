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
				if o.Output == nil || *o.Output != "json" {
					t.Errorf("output = %v, want json", o.Output)
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var o scan.Overlay
			if err := yaml.Unmarshal([]byte(tc.yamlData), &o); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}
			tc.verify(t, o)
		})
	}
}

func TestApply(t *testing.T) {
	cfg := config.Default()

	// Initial values verify
	if cfg.Threads != 32 {
		t.Fatalf("expected default threads 32, got %d", cfg.Threads)
	}
	if cfg.Recursive {
		t.Fatalf("expected default recursive false")
	}

	// 1. Apply single profile overrides
	threadsVal := 64
	recursiveVal := true
	o1 := scan.Overlay{
		Threads:   &threadsVal,
		Recursive: &recursiveVal,
	}

	scan.Apply(&cfg, o1)

	if cfg.Threads != 64 {
		t.Errorf("threads = %d, want 64", cfg.Threads)
	}
	if !cfg.Recursive {
		t.Errorf("recursive = false, want true")
	}

	// 2. Verify omitted fields remain unchanged (e.g. strategy should still be BFS)
	if cfg.Strategy != recursion.BFS {
		t.Errorf("strategy = %v, want BFS", cfg.Strategy)
	}

	// 3. Apply multiple profiles (precedence)
	threadsVal2 := 12
	strategyVal := "dfs"
	o2 := scan.Overlay{
		Threads:  &threadsVal2,
		Strategy: &strategyVal,
	}

	scan.Apply(&cfg, o2)

	if cfg.Threads != 12 {
		t.Errorf("threads = %d, want 12 (overridden by second profile)", cfg.Threads)
	}
	if cfg.Strategy != recursion.DFS {
		t.Errorf("strategy = %v, want DFS (updated by second profile)", cfg.Strategy)
	}
}
