package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/unsubble/searchit/internal/config"
)

func TestCLI_Validation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "missing url",
			args:    []string{"scan"},
			wantErr: true,
		},
		{
			name:    "valid basic url",
			args:    []string{"scan", "-u", "http://localhost"},
			wantErr: false,
		},
		{
			name:    "invalid threads",
			args:    []string{"scan", "-u", "http://localhost", "-t", "0"},
			wantErr: true,
		},
		{
			name:    "invalid strategy",
			args:    []string{"scan", "-u", "http://localhost", "--strategy", "invalid"},
			wantErr: true,
		},
		{
			name:    "max-depth without recursive",
			args:    []string{"scan", "-u", "http://localhost", "--max-depth", "5"},
			wantErr: true,
		},
		{
			name:    "max-depth with recursive",
			args:    []string{"scan", "-u", "http://localhost", "-r", "--max-depth", "5"},
			wantErr: false,
		},
		{
			name:    "invalid max-depth with recursive",
			args:    []string{"scan", "-u", "http://localhost", "-r", "--max-depth", "0"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flagURL = ""
			flagWordlist = ""
			flagThreads = 32
			flagTimeout = 10
			flagRecursive = false
			flagMaxDepth = 3
			flagStrategy = "bfs"
			flagExcludeStatus = "404"

			cmd := rootCmd
			cmd.SetArgs(tc.args)

			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := cmd.ExecuteContext(ctx)
			if (err != nil) != tc.wantErr {
				t.Errorf("args %v: error = %v, wantErr %v", tc.args, err, tc.wantErr)
			}
		})
	}
}

func TestEmbeddedWordlistFallback(t *testing.T) {
	cfg := config.Default()
	if cfg.Wordlist != "" {
		t.Errorf("default Wordlist = %q, want empty (which triggers embedded common.txt fallback)", cfg.Wordlist)
	}
}
