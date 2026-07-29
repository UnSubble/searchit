package cmd

import (
	"reflect"
	"testing"
)

func TestProfileParsing(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "-p performance",
			args:     []string{"--profile", "performance"},
			expected: []string{"performance"},
		},
		{
			name:     "-p performance,proxy",
			args:     []string{"--profile", "performance,proxy"},
			expected: []string{"performance", "proxy"},
		},
		{
			name:     "-p performance -p proxy",
			args:     []string{"--profile", "performance", "--profile", "proxy"},
			expected: []string{"proxy"},
		},
		{
			name:     "-p performance,proxy -p auth",
			args:     []string{"--profile", "performance,proxy", "--profile", "auth"},
			expected: []string{"auth"},
		},
		{
			name:     "-p performance -p proxy,auth",
			args:     []string{"--profile", "performance", "--profile", "proxy,auth"},
			expected: []string{"proxy", "auth"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, opts := NewScanCmd()
			cmd.SetArgs(tt.args)

			if err := cmd.ParseFlags(tt.args); err != nil {
				t.Fatalf("ParseFlags failed: %v", err)
			}

			opts.Threads = 32
			opts.MaxDepth = 3
			opts.Strategy = "bfs"

			_ = cmd.PreRunE(cmd, tt.args)

			if len(opts.Profiles) == 0 && len(tt.expected) == 0 {
				return
			}
			if !reflect.DeepEqual(opts.Profiles, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, opts.Profiles)
			}
		})
	}
}
