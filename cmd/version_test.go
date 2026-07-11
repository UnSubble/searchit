package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestVersionCmd_Output(t *testing.T) {
	rootCmd.SetContext(context.Background())
	versionCmd.SetContext(context.Background())

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	versionCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.SetArgs([]string{"version"})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	ctx := context.Background()
	err := cmd.ExecuteContext(ctx)
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "searchit v") {
		t.Errorf("expected version output to contain 'searchit v', got %q", out)
	}
	if !strings.Contains(out, "Commit:") {
		t.Errorf("expected version output to contain 'Commit:', got %q", out)
	}
	if !strings.Contains(out, "Built:") {
		t.Errorf("expected version output to contain 'Built:', got %q", out)
	}
}

func TestVersionFlag_Output(t *testing.T) {
	rootCmd.SetContext(context.Background())

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.SetArgs([]string{"--version"})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	ctx := context.Background()
	err := cmd.ExecuteContext(ctx)
	if err != nil {
		t.Fatalf("version flag failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "searchit v") {
		t.Errorf("expected version flag output to contain 'searchit v', got %q", out)
	}
	if strings.Contains(out, "Commit:") {
		t.Errorf("expected short version flag output to NOT contain 'Commit:', got %q", out)
	}
}
