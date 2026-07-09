package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/unsubble/searchit/internal/config"
)

func resetFlagsForTest() {
	flagURL = ""
	flagURLFile = ""
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
	flagOutput = "text"
	flagQuiet = false
	flagIncludeSize = ""
	flagExcludeSize = ""
	flagIncludeHeaders = nil
	flagExcludeHeaders = nil
	flagDelay = ""
	flagRate = 0
	flagConnectTimeout = "3s"
	flagProfiles = nil
	flagProgress = false
	resolvedTargets = nil
}

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
			name:    "invalid output mode",
			args:    []string{"scan", "-u", "http://localhost", "--output", "invalid"},
			wantErr: true,
		},
		{
			name:    "explicit text output",
			args:    []string{"scan", "-u", "http://localhost", "--output", "text"},
			wantErr: false,
		},
		{
			name:    "explicit json output",
			args:    []string{"scan", "-u", "http://localhost", "--output", "json"},
			wantErr: false,
		},
		{
			name:    "explicit ndjson output",
			args:    []string{"scan", "-u", "http://localhost", "--output", "ndjson"},
			wantErr: false,
		},
		{
			name:    "invalid include-size format",
			args:    []string{"scan", "-u", "http://localhost", "--include-size", "abc"},
			wantErr: true,
		},
		{
			name:    "invalid exclude-size range bounds",
			args:    []string{"scan", "-u", "http://localhost", "--exclude-size", "200-100"},
			wantErr: true,
		},
		{
			name:    "invalid include-header missing equal",
			args:    []string{"scan", "-u", "http://localhost", "--include-header", "Server"},
			wantErr: true,
		},
		{
			name:    "invalid exclude-header empty value",
			args:    []string{"scan", "-u", "http://localhost", "--exclude-header", "Server="},
			wantErr: true,
		},
		{
			name:    "invalid include-header empty name",
			args:    []string{"scan", "-u", "http://localhost", "--include-header", "=nginx"},
			wantErr: true,
		},
		{
			name:    "valid include-header and exclude-size",
			args:    []string{"scan", "-u", "http://localhost", "--include-header", "Server=nginx", "--exclude-size", "0,123"},
			wantErr: false,
		},
		{
			name:    "valid quiet mode option long-form",
			args:    []string{"scan", "-u", "http://localhost", "--quiet"},
			wantErr: false,
		},
		{
			name:    "valid quiet mode option shorthand",
			args:    []string{"scan", "-u", "http://localhost", "-q"},
			wantErr: false,
		},
		{
			name:    "empty target URL and URL file",
			args:    []string{"scan"},
			wantErr: true,
		},
		{
			name:    "missing URL file",
			args:    []string{"scan", "--url-file", "nonexistent.txt"},
			wantErr: true,
		},
		{
			name:    "multiple target URLs via comma-separated list",
			args:    []string{"scan", "-u", "http://a.com,http://b.com"},
			wantErr: false,
		},
		{
			name:    "valid delay 100ms",
			args:    []string{"scan", "-u", "http://localhost", "--delay", "100ms"},
			wantErr: false,
		},
		{
			name:    "valid delay 1s",
			args:    []string{"scan", "-u", "http://localhost", "--delay", "1s"},
			wantErr: false,
		},
		{
			name:    "invalid delay format",
			args:    []string{"scan", "-u", "http://localhost", "--delay", "abc"},
			wantErr: true,
		},
		{
			name:    "valid rate float",
			args:    []string{"scan", "-u", "http://localhost", "--rate", "25.5"},
			wantErr: false,
		},
		{
			name:    "invalid rate float format",
			args:    []string{"scan", "-u", "http://localhost", "--rate", "abc"},
			wantErr: true,
		},
		{
			name:    "negative rate float",
			args:    []string{"scan", "-u", "http://localhost", "--rate", "-5.5"},
			wantErr: true,
		},
		{
			name:    "zero rate float explicitly passed",
			args:    []string{"scan", "-u", "http://localhost", "--rate", "0"},
			wantErr: true,
		},
		{
			name:    "valid connect-timeout",
			args:    []string{"scan", "-u", "http://localhost", "--connect-timeout", "500ms"},
			wantErr: false,
		},
		{
			name:    "invalid connect-timeout format",
			args:    []string{"scan", "-u", "http://localhost", "--connect-timeout", "abc"},
			wantErr: true,
		},
		{
			name:    "negative connect-timeout",
			args:    []string{"scan", "-u", "http://localhost", "--connect-timeout", "-5s"},
			wantErr: true,
		},
		{
			name:    "zero connect-timeout",
			args:    []string{"scan", "-u", "http://localhost", "--connect-timeout", "0"},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rootCmd.SetContext(nil)
			scanCmd.SetContext(nil)
			resetFlagsForTest()

			cmd := rootCmd
			cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
			scanCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
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
			name: "quiet mode with recurse-on prints startup messages",
			args: []string{"scan", "-u", "http://localhost", "-r", "--quiet"},
			wantPrints: []string{
				"[*] Recursive scan enabled",
				"[*] Strategy: bfs",
				"[*] Max depth: 3",
				"[*] Recurse on: 200,301,302,403",
				"[*] 404 responses are ignored by default.",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rootCmd.SetContext(nil)
			scanCmd.SetContext(nil)
			resetFlagsForTest()

			cmd := rootCmd
			cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
			scanCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
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
	rootCmd.SetContext(nil)
	scanCmd.SetContext(nil)
	resetFlagsForTest()

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	scanCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
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

func TestCLI_ShorthandsValueBinding(t *testing.T) {
	rootCmd.SetContext(nil)
	scanCmd.SetContext(nil)
	resetFlagsForTest()

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	scanCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.SetArgs([]string{
		"scan",
		"-u", "http://localhost",
		"-w", "my-wordlist.txt",
		"-t", "64",
		"-r",
		"-d", "5",
		"-s", "dfs",
		"-x", "404,500",
		"-o", "ndjson",
		"-H", "Server=nginx",
		"-H", "X-Header=val",
	})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = cmd.ExecuteContext(ctx)

	if flagURL != "http://localhost" {
		t.Errorf("expected flagURL='http://localhost', got %q", flagURL)
	}
	if flagWordlist != "my-wordlist.txt" {
		t.Errorf("expected flagWordlist='my-wordlist.txt', got %q", flagWordlist)
	}
	if flagThreads != 64 {
		t.Errorf("expected flagThreads=64, got %d", flagThreads)
	}
	if !flagRecursive {
		t.Error("expected flagRecursive to be true")
	}
	if flagMaxDepth != 5 {
		t.Errorf("expected flagMaxDepth=5, got %d", flagMaxDepth)
	}
	if flagStrategy != "dfs" {
		t.Errorf("expected flagStrategy='dfs', got %q", flagStrategy)
	}
	if flagExcludeStatus != "404,500" {
		t.Errorf("expected flagExcludeStatus='404,500', got %q", flagExcludeStatus)
	}
	if flagOutput != "ndjson" {
		t.Errorf("expected flagOutput='ndjson', got %q", flagOutput)
	}
	if len(flagIncludeHeaders) != 2 || flagIncludeHeaders[0] != "Server=nginx" || flagIncludeHeaders[1] != "X-Header=val" {
		t.Errorf("expected flagIncludeHeaders=[Server=nginx, X-Header=val], got %v", flagIncludeHeaders)
	}
}

func TestCLI_QuietMode_StartupPrints(t *testing.T) {
	rootCmd.SetContext(nil)
	scanCmd.SetContext(nil)
	resetFlagsForTest()

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	scanCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.SetArgs([]string{"scan", "-u", "http://localhost", "-r", "--quiet"})

	// Capture stdout
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

	if !strings.Contains(gotOut, "[*] Recursive scan enabled") {
		t.Error("expected quiet mode to keep recursive scan startup message")
	}
}

func TestCLI_MultipleTargetsAndFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "targets.txt")
	content := "http://b.com\nhttp://c.com\n"
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	rootCmd.SetContext(nil)
	scanCmd.SetContext(nil)
	resetFlagsForTest()

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	scanCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.SetArgs([]string{"scan", "-u", "http://a.com", "--url-file", filePath})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = cmd.ExecuteContext(ctx)

	wantTargets := []string{"http://a.com", "http://b.com", "http://c.com"}
	if !reflect.DeepEqual(resolvedTargets, wantTargets) {
		t.Errorf("resolvedTargets = %v, want %v", resolvedTargets, wantTargets)
	}
}

func TestCLI_ProgressFlags(t *testing.T) {
	rootCmd.SetContext(nil)
	scanCmd.SetContext(nil)
	resetFlagsForTest()

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	scanCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"scan", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("help execute failed: %v", err)
	}

	helpOut := buf.String()
	if !strings.Contains(helpOut, "--progress") {
		t.Errorf("expected help output to contain --progress flag, but got:\n%s", helpOut)
	}
	if !strings.Contains(helpOut, "quiet") || !strings.Contains(helpOut, "json") {
		t.Errorf("expected help output to mention quiet mode or json output disabling conditions, but got:\n%s", helpOut)
	}
}
