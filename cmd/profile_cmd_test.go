package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func runProfileCommand(args []string) (string, error) {
	rootCmd.SetContext(nil)
	profileCmd.SetContext(nil)
	profileListCmd.SetContext(nil)
	profileShowCmd.SetContext(nil)

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	profileCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	profileListCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	profileShowCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.SetArgs(args)

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	ctx := context.Background()
	err := cmd.ExecuteContext(ctx)
	return buf.String(), err
}

func TestProfileListOutput(t *testing.T) {
	out, err := runProfileCommand([]string{"profile", "list"})
	if err != nil {
		t.Fatalf("profile list: %v", err)
	}

	if !strings.Contains(out, "BUILTIN") {
		t.Errorf("output missing BUILTIN header; got:\n%s", out)
	}

	wantProfiles := []string{"scan/base", "scan/quick", "scan/deep"}
	for _, want := range wantProfiles {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestProfileShowOutput(t *testing.T) {
	out, err := runProfileCommand([]string{"profile", "show", "scan/quick"})
	if err != nil {
		t.Fatalf("profile show scan/quick: %v", err)
	}

	// Should contain YAML content.
	wantFields := []string{"name:", "scan/quick", "tool:", "scan", "threads:"}
	for _, want := range wantFields {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestProfileShowMissing(t *testing.T) {
	_, err := runProfileCommand([]string{"profile", "show", "nonexistent"})
	if err == nil {
		t.Fatal("profile show nonexistent: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

func TestProfileShowAllEmbedded(t *testing.T) {
	names := []string{"scan/base", "scan/quick", "scan/deep"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			out, err := runProfileCommand([]string{"profile", "show", name})
			if err != nil {
				t.Fatalf("profile show %s: %v", name, err)
			}
			if !strings.Contains(out, "name: "+name) {
				t.Errorf("output missing 'name: %s'; got:\n%s", name, out)
			}
		})
	}
}

func TestProfileNoCreateCommand(t *testing.T) {
	// Verify that the old "profile create" command is not a registered subcommand.
	for _, sub := range profileCmd.Commands() {
		if sub.Name() == "create" {
			t.Fatal("profile create subcommand should not be registered")
		}
	}
}
