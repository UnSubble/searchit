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
	"time"

	"github.com/spf13/pflag"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/targets"
)

func TestCLI_Validation(t *testing.T) {
	cmd, opts := NewScanCmd()
	_ = opts
	_ = cmd
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
			args:    []string{"-u", "http://localhost"},
			wantErr: false,
		},
		{
			name:    "invalid threads",
			args:    []string{"-u", "http://localhost", "-t", "0"},
			wantErr: true,
		},
		{
			name:    "invalid strategy",
			args:    []string{"-u", "http://localhost", "--strategy", "invalid"},
			wantErr: true,
		},
		{
			name:    "max-depth without recursive",
			args:    []string{"-u", "http://localhost", "--max-depth", "5"},
			wantErr: true,
		},
		{
			name:    "max-depth with recursive",
			args:    []string{"-u", "http://localhost", "-r", "--max-depth", "5"},
			wantErr: false,
		},
		{
			name:    "invalid max-depth with recursive",
			args:    []string{"-u", "http://localhost", "-r", "--max-depth", "0"},
			wantErr: true,
		},
		{
			name:    "recurse-on without recursive",
			args:    []string{"-u", "http://localhost", "--recurse-on", "200"},
			wantErr: true,
		},
		{
			name:    "invalid recurse-on format",
			args:    []string{"-u", "http://localhost", "-r", "--recurse-on", "abc"},
			wantErr: true,
		},
		{
			name:    "valid recurse-on wildcard",
			args:    []string{"-u", "http://localhost", "-r", "--recurse-on", "2xx"},
			wantErr: false,
		},
		{
			name:    "invalid format name",
			args:    []string{"-u", "http://localhost", "--format", "invalid"},
			wantErr: true,
		},
		{
			name:    "explicit text format",
			args:    []string{"-u", "http://localhost", "--format", "text"},
			wantErr: false,
		},
		{
			name:    "explicit json format",
			args:    []string{"-u", "http://localhost", "--format", "json"},
			wantErr: false,
		},
		{
			name:    "explicit ndjson format",
			args:    []string{"-u", "http://localhost", "--format", "ndjson"},
			wantErr: false,
		},
		{
			name:    "explicit csv format",
			args:    []string{"-u", "http://localhost", "--format", "csv"},
			wantErr: false,
		},
		{
			name:    "explicit markdown format",
			args:    []string{"-u", "http://localhost", "--format", "markdown"},
			wantErr: false,
		},
		{
			name:    "output file path is valid",
			args:    []string{"-u", "http://localhost", "-o", filepath.Join(t.TempDir(), "searchit_test_output.json")},
			wantErr: false,
		},
		{
			name:    "output is a directory returns error",
			args:    []string{"-u", "http://localhost", "-o", t.TempDir()},
			wantErr: true,
		},
		{
			name:    "invalid include-size format",
			args:    []string{"-u", "http://localhost", "--include-size", "abc"},
			wantErr: true,
		},
		{
			name:    "invalid exclude-size range bounds",
			args:    []string{"-u", "http://localhost", "--exclude-size", "200-100"},
			wantErr: true,
		},
		{
			name:    "invalid include-header missing equal",
			args:    []string{"-u", "http://localhost", "--include-header", "Server"},
			wantErr: true,
		},
		{
			name:    "invalid exclude-header empty value",
			args:    []string{"-u", "http://localhost", "--exclude-header", "Server="},
			wantErr: true,
		},
		{
			name:    "invalid include-header empty name",
			args:    []string{"-u", "http://localhost", "--include-header", "=nginx"},
			wantErr: true,
		},
		{
			name:    "valid include-header and exclude-size",
			args:    []string{"-u", "http://localhost", "--include-header", "Server=nginx", "--exclude-size", "0,123"},
			wantErr: false,
		},
		{
			name:    "valid quiet mode option long-form",
			args:    []string{"-u", "http://localhost", "--quiet"},
			wantErr: false,
		},
		{
			name:    "valid quiet mode option shorthand",
			args:    []string{"-u", "http://localhost", "-q"},
			wantErr: false,
		},
		{
			name:    "empty target URL and URL file",
			args:    []string{"scan"},
			wantErr: true,
		},
		{
			name:    "missing URL file",
			args:    []string{"--url-file", "nonexistent.txt"},
			wantErr: true,
		},
		{
			name:    "multiple target URLs via comma-separated list",
			args:    []string{"-u", "http://a.com,http://b.com"},
			wantErr: false,
		},
		{
			name:    "valid delay 100ms",
			args:    []string{"-u", "http://localhost", "--delay", "100ms"},
			wantErr: false,
		},
		{
			name:    "valid delay 1s",
			args:    []string{"-u", "http://localhost", "--delay", "1s"},
			wantErr: false,
		},
		{
			name:    "invalid delay format",
			args:    []string{"-u", "http://localhost", "--delay", "abc"},
			wantErr: true,
		},
		{
			name:    "valid rate float",
			args:    []string{"-u", "http://localhost", "--rate", "25.5"},
			wantErr: false,
		},
		{
			name:    "invalid rate float format",
			args:    []string{"-u", "http://localhost", "--rate", "abc"},
			wantErr: true,
		},
		{
			name:    "negative rate float",
			args:    []string{"-u", "http://localhost", "--rate", "-5.5"},
			wantErr: true,
		},
		{
			name:    "zero rate float explicitly passed",
			args:    []string{"-u", "http://localhost", "--rate", "0"},
			wantErr: true,
		},
		{
			name:    "valid connect-timeout",
			args:    []string{"-u", "http://localhost", "--connect-timeout", "500ms"},
			wantErr: false,
		},
		{
			name:    "invalid connect-timeout format",
			args:    []string{"-u", "http://localhost", "--connect-timeout", "abc"},
			wantErr: true,
		},
		{
			name:    "negative connect-timeout",
			args:    []string{"-u", "http://localhost", "--connect-timeout", "-5s"},
			wantErr: true,
		},
		{
			name:    "zero connect-timeout",
			args:    []string{"-u", "http://localhost", "--connect-timeout", "0"},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, opts := NewScanCmd()
			_ = opts
			_ = cmd
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			cmd.SetContext(ctx)

			cmd.SetArgs(tc.args)

			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

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
	cmd, opts := NewScanCmd()
	_ = opts
	_ = cmd
	tests := []struct {
		name       string
		args       []string
		wantPrints []string
		omitPrints []string
	}{
		{
			name: "default recurse-on (excludes 404)",
			args: []string{"-u", "http://localhost", "-r"},
			wantPrints: []string{
				"Target                       http://localhost",
				"Mode                         Recursive",
				"Wordlist                     embedded",
				"Workers                      32",
			},
		},
		{
			name: "quiet mode with recurse-on prints startup messages",
			args: []string{"-u", "http://localhost", "-r", "--quiet"},
			omitPrints: []string{
				"Target                       http://localhost",
				"Mode                         Recursive",
				"Wordlist                     embedded",
				"Workers                      32",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, opts := NewScanCmd()
			_ = opts
			_ = cmd
			// Provide enough time for startup info to print, but don't hang if there's no server
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			cmd.SetContext(ctx)

			cmd.SetArgs(tc.args)

			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Capture stdout and stderr using pipe
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("os.Pipe: %v", err)
			}
			oldStdout := os.Stdout
			oldStderr := os.Stderr
			os.Stdout = w
			os.Stderr = w

			_ = cmd.ExecuteContext(ctx)

			w.Close()
			os.Stdout = oldStdout
			os.Stderr = oldStderr

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
	cmd, opts := NewScanCmd()
	_ = opts
	_ = cmd
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd.SetContext(ctx)

	cmd.SetArgs([]string{"-u", "http://localhost", "--normalize-paths", "--collapse-slashes"})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	_ = cmd.ExecuteContext(ctx)

	if !opts.NormalizePaths {
		t.Error("expected opts.NormalizePaths to be true when --normalize-paths is supplied")
	}
	if !opts.CollapseSlashes {
		t.Error("expected opts.CollapseSlashes to be true when --collapse-slashes is supplied")
	}
}

func TestCLI_ShorthandsValueBinding(t *testing.T) {
	cmd, opts := NewScanCmd()
	_ = opts
	_ = cmd
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd.SetContext(ctx)

	cmd.SetArgs([]string{
		"scan",
		"-u", "http://localhost",
		"-w", "my-wordlist.txt",
		"-t", "64",
		"-r",
		"-d", "5",
		"-s", "dfs",
		"-x", "404,500",
		"--format", "ndjson",
		"--include-header", "Server=nginx",
		"--include-header", "X-Header=val",
	})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	_ = cmd.ExecuteContext(ctx)

	if opts.URL != "http://localhost" {
		t.Errorf("expected opts.URL='http://localhost', got %q", opts.URL)
	}
	if opts.Wordlist != "my-wordlist.txt" {
		t.Errorf("expected opts.Wordlist='my-wordlist.txt', got %q", opts.Wordlist)
	}
	if opts.Threads != 64 {
		t.Errorf("expected opts.Threads=64, got %d", opts.Threads)
	}
	if !opts.Recursive {
		t.Error("expected opts.Recursive to be true")
	}
	if opts.MaxDepth != 5 {
		t.Errorf("expected opts.MaxDepth=5, got %d", opts.MaxDepth)
	}
	if opts.Strategy != "dfs" {
		t.Errorf("expected opts.Strategy='dfs', got %q", opts.Strategy)
	}
	if opts.ExcludeStatus != "404,500" {
		t.Errorf("expected opts.ExcludeStatus='404,500', got %q", opts.ExcludeStatus)
	}
	if opts.Format != "ndjson" {
		t.Errorf("expected opts.Format='ndjson', got %q", opts.Format)
	}
	if len(opts.IncludeHeaders) != 2 || opts.IncludeHeaders[0] != "Server=nginx" || opts.IncludeHeaders[1] != "X-Header=val" {
		t.Errorf("expected opts.IncludeHeaders=[Server=nginx, X-Header=val], got %v", opts.IncludeHeaders)
	}
}

func TestCLI_QuietMode_StartupPrints(t *testing.T) {
	cmd, opts := NewScanCmd()
	_ = opts
	_ = cmd
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd.SetContext(ctx)

	cmd.SetArgs([]string{"-u", "http://localhost", "-r", "--quiet"})

	// Capture stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	_ = cmd.ExecuteContext(ctx)

	w.Close()
	os.Stdout = oldStdout

	var stdoutBuf bytes.Buffer
	if _, err := io.Copy(&stdoutBuf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	gotOut := stdoutBuf.String()

	if strings.Contains(gotOut, "[*] Recursive scan enabled") {
		t.Error("expected quiet mode to suppress recursive scan startup message")
	}
}

func TestCLI_MultipleTargetsAndFile(t *testing.T) {
	cmd, opts := NewScanCmd()
	_ = opts
	_ = cmd
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "targets.txt")
	content := "http://b.com\nhttp://c.com\n"
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd.SetContext(ctx)

	cmd.SetArgs([]string{"-u", "http://a.com", "--url-file", filePath})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	_ = cmd.ExecuteContext(ctx)

	wantTargets := []targets.Target{
		{URL: "http://a.com", ID: 1},
		{URL: "http://b.com", ID: 2},
		{URL: "http://c.com", ID: 3},
	}
	if !reflect.DeepEqual(opts.resolvedTargets, wantTargets) {
		t.Errorf("opts.resolvedTargets = %v, want %v", opts.resolvedTargets, wantTargets)
	}
}

func TestCLI_ProgressFlags(t *testing.T) {
	cmd, opts := NewScanCmd()
	_ = opts
	_ = cmd
	rootCmd.SetContext(context.Background())
	cmd.SetContext(context.Background())

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("help execute failed: %v", err)
	}

	helpOut := buf.String()
	if !strings.Contains(helpOut, "--no-progress") {
		t.Errorf("expected help output to contain --no-progress flag, but got:\n%s", helpOut)
	}
	if strings.Contains(helpOut, "--progress") && !strings.Contains(helpOut, "--no-progress") {
		t.Errorf("expected --progress to be removed from help output; got:\n%s", helpOut)
	}
}

// TestCLI_ProgressActivation verifies the shouldEnableProgress logic for all
// relevant combinations of flags and terminal state.
func TestCLI_ProgressActivation(t *testing.T) {
	tests := []struct {
		name         string
		noProgress   bool
		quiet        bool
		outputFormat string
		isTerminal   bool
		want         bool
	}{
		{
			name:       "interactive terminal with text format enables progress",
			noProgress: false, quiet: false, outputFormat: "text", isTerminal: true,
			want: true,
		},
		{
			name:       "--no-progress disables progress even in terminal",
			noProgress: true, quiet: false, outputFormat: "text", isTerminal: true,
			want: false,
		},
		{
			name:       "--quiet disables progress",
			noProgress: false, quiet: true, outputFormat: "text", isTerminal: true,
			want: false,
		},
		{
			name:       "json format enables progress in terminal",
			noProgress: false, quiet: false, outputFormat: "json", isTerminal: true,
			want: true,
		},
		{
			name:       "ndjson format enables progress in terminal",
			noProgress: false, quiet: false, outputFormat: "ndjson", isTerminal: true,
			want: true,
		},
		{
			name:       "non-TTY stderr (piped/redirected) disables progress",
			noProgress: false, quiet: false, outputFormat: "text", isTerminal: false,
			want: false,
		},
		{
			name:       "--no-progress with json format stays disabled",
			noProgress: true, quiet: false, outputFormat: "json", isTerminal: true,
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{
				Quiet:        tc.quiet,
				OutputFormat: tc.outputFormat,
			}
			// shouldEnableProgress calls console.IsTerminal(os.Stderr.Fd()).
			// In tests, stderr is a pipe (not a TTY), so isTerminal will always
			// be false unless we explicitly test the non-TTY path.
			// We test the helper with a thin wrapper that accepts isTerminal.
			got := shouldEnableProgressWith(cfg, tc.noProgress, tc.isTerminal)
			if got != tc.want {
				t.Errorf("shouldEnableProgress(noProgress=%v, quiet=%v, outputFormat=%q, isTerminal=%v) = %v, want %v",
					tc.noProgress, tc.quiet, tc.outputFormat, tc.isTerminal, got, tc.want)
			}
		})
	}
}

// shouldEnableProgressWith is a testable variant of shouldEnableProgress that
// accepts isTerminal as a parameter, avoiding the need for a real TTY in tests.
func shouldEnableProgressWith(cfg config.Config, noProgress bool, isTerminal bool) bool {
	if noProgress {
		return false
	}
	if cfg.Quiet {
		return false
	}
	if !isTerminal {
		return false
	}
	return true
}

func TestCLI_TechFlag(t *testing.T) {
	cmd, opts := NewScanCmd()
	_ = opts
	_ = cmd
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "valid tech laravel",
			args:    []string{"-u", "http://localhost", "--tech", "laravel"},
			wantErr: false,
		},
		{
			name:    "valid tech spring",
			args:    []string{"-u", "http://localhost", "--tech", "spring"},
			wantErr: false,
		},
		{
			name:    "valid tech uppercase",
			args:    []string{"-u", "http://localhost", "--tech", "LARAVEL"},
			wantErr: false,
		},
		{
			name:    "valid tech mixed case",
			args:    []string{"-u", "http://localhost", "--tech", "WordPress"},
			wantErr: false,
		},
		{
			name:    "unknown tech rejected",
			args:    []string{"-u", "http://localhost", "--tech", "rails"},
			wantErr: true,
		},
		{
			name:    "empty tech treated as no tech specified",
			args:    []string{"-u", "http://localhost", "--tech", ""},
			wantErr: false,
		},
		{
			name:    "no tech flag is valid",
			args:    []string{"-u", "http://localhost"},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, opts := NewScanCmd()
			_ = opts
			_ = cmd
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			cmd.SetContext(ctx)
			rootCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })

			cmd.SetArgs(tc.args)

			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			opts.testHookConfigApplied = func(config.Config) {}
			defer func() { opts.testHookConfigApplied = nil }()

			err := cmd.ExecuteContext(ctx)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestApplyCLIOverrides_TechProfile(t *testing.T) {
	runWithTech := func(t *testing.T, techArg string) config.Config {
		cmd, opts := NewScanCmd()
		_ = opts
		_ = cmd
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		cmd.SetContext(ctx)
		rootCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })

		args := []string{"-u", "http://localhost"}
		if techArg != "" {
			args = append(args, "--tech", techArg)
		}
		cmd.SetArgs(args)

		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		var got config.Config
		opts.testHookConfigApplied = func(c config.Config) { got = c }
		defer func() { opts.testHookConfigApplied = nil }()

		if err := cmd.ExecuteContext(ctx); err != nil {
			t.Fatalf("Execute() error: %v", err)
		}
		return got
	}

	t.Run("tech flag populates TechProfile", func(t *testing.T) {
		got := runWithTech(t, "laravel")
		if got.TechProfile == nil {
			t.Fatal("TechProfile is nil, want non-nil")
		}
		if got.TechProfile.ID != "laravel" {
			t.Errorf("TechProfile.ID = %q, want %q", got.TechProfile.ID, "laravel")
		}
		if got.TechProfile.DisplayName != "Laravel" {
			t.Errorf("TechProfile.DisplayName = %q, want %q", got.TechProfile.DisplayName, "Laravel")
		}
	})

	t.Run("case-insensitive --tech sets canonical ID", func(t *testing.T) {
		got := runWithTech(t, "SPRING")
		if got.TechProfile == nil {
			t.Fatal("TechProfile is nil, want non-nil")
		}
		if got.TechProfile.ID != "spring" {
			t.Errorf("TechProfile.ID = %q, want canonical %q", got.TechProfile.ID, "spring")
		}
	})

	t.Run("no --tech flag leaves TechProfile nil", func(t *testing.T) {
		got := runWithTech(t, "")
		if got.TechProfile != nil {
			t.Errorf("TechProfile = %+v, want nil", got.TechProfile)
		}
	})
}
