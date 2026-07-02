package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
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
		{
			name:    "recurse-on without recursive",
			args:    []string{"scan", "-u", "http://localhost", "--recurse-on", "200"},
			wantErr: true,
		},
		{
			name:    "invalid recurse-on format",
			args:    []string{"scan", "-u", "http://localhost", "-r", "--recurse-on", "abc"},
			wantErr: true,
		},
		{
			name:    "valid recurse-on wildcard",
			args:    []string{"scan", "-u", "http://localhost", "-r", "--recurse-on", "2xx"},
			wantErr: false,
		},
		{
			name:    "valid recurse-on exact",
			args:    []string{"scan", "-u", "http://localhost", "-r", "--recurse-on", "404"},
			wantErr: false,
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
			flagRecurseOn = "200,301,302,403"

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

func TestCLI_StartupInformation(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantPrints []string
		omitPrints []string
	}{
		{
			name: "default recurse-on (excludes 404)",
			args: []string{"scan", "-u", "http://localhost", "-r"},
			wantPrints: []string{
				"[*] Recursive scan enabled",
				"[*] Strategy: bfs",
				"[*] Max depth: 3",
				"[*] Recurse on: 200,301,302,403",
				"[*] 404 responses are ignored by default.",
			},
		},
		{
			name: "recurse-on including 404",
			args: []string{"scan", "-u", "http://localhost", "-r", "--recurse-on", "404"},
			wantPrints: []string{
				"[*] Recursive scan enabled",
				"[*] Recurse on: 404",
			},
			omitPrints: []string{
				"[*] 404 responses are ignored by default.",
			},
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
			flagRecurseOn = "200,301,302,403"

			cmd := rootCmd
			cmd.SetArgs(tc.args)

			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Capture stdout using pipe
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("os.Pipe: %v", err)
			}
			oldStdout := os.Stdout
			os.Stdout = w

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_ = cmd.ExecuteContext(ctx)

			w.Close()
			os.Stdout = oldStdout

			var stdoutBuf bytes.Buffer
			if _, err := io.Copy(&stdoutBuf, r); err != nil {
				t.Fatalf("io.Copy: %v", err)
			}
			gotOut := stdoutBuf.String()

			for _, want := range tc.wantPrints {
				if !strings.Contains(gotOut, want) {
					t.Errorf("expected output to contain %q, but got:\n%s", want, gotOut)
				}
			}
			for _, omit := range tc.omitPrints {
				if strings.Contains(gotOut, omit) {
					t.Errorf("expected output to NOT contain %q, but got:\n%s", omit, gotOut)
				}
			}
		})
	}
}

func TestCLI_PathFlags(t *testing.T) {
	flagURL = ""
	flagWordlist = ""
	flagThreads = 32
	flagTimeout = 10
	flagRecursive = false
	flagMaxDepth = 3
	flagStrategy = "bfs"
	flagExcludeStatus = "404"
	flagRecurseOn = "200,301,302,403"
	flagNormalizePaths = false
	flagCollapseSlashes = false

	cmd := rootCmd
	cmd.SetArgs([]string{"scan", "-u", "http://localhost", "--normalize-paths", "--collapse-slashes"})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = cmd.ExecuteContext(ctx)

	if !flagNormalizePaths {
		t.Error("expected flagNormalizePaths to be true when --normalize-paths is supplied")
	}
	if !flagCollapseSlashes {
		t.Error("expected flagCollapseSlashes to be true when --collapse-slashes is supplied")
	}
}
