package cmd

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/profile"
	"gopkg.in/yaml.v3"
)

func TestProfileStacking_ScanMultipleProfilesPrecedence(t *testing.T) {
	p1 := profile.Profile{
		Schema: 1,
		Name:   "scan/stack1",
		Tool:   "scan",
		Config: yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "threads"},
				{Kind: yaml.ScalarNode, Tag: "!!int", Value: "10"},
				{Kind: yaml.ScalarNode, Value: "timeout"},
				{Kind: yaml.ScalarNode, Tag: "!!int", Value: "5"},
			},
		},
	}
	p2 := profile.Profile{
		Schema: 1,
		Name:   "scan/stack2",
		Tool:   "scan",
		Config: yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "threads"},
				{Kind: yaml.ScalarNode, Tag: "!!int", Value: "20"},
				{Kind: yaml.ScalarNode, Value: "proxy"},
				{Kind: yaml.ScalarNode, Tag: "!!str", Value: "http://proxy"},
			},
		},
	}

	store := profile.NewStore()
	store.Create(p1)
	store.Create(p2)

	home, _ := os.UserHomeDir()
	defer os.RemoveAll(home + "/.config/searchit/profiles/scan/stack1.yaml")
	defer os.RemoveAll(home + "/.config/searchit/profiles/scan/stack2.yaml")

	var captured config.Config
	cmd, opts := NewScanCmd()
	opts.URL = "http://localhost"
	opts.testHookConfigApplied = func(cfg config.Config) {
		captured = cfg
	}
	cmd.SetArgs([]string{"scan", "--profile", "scan/stack1,scan/stack2"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("scan command failed: %v", err)
	}

	if captured.Threads != 20 {
		t.Errorf("expected threads 20 (from stack2), got %d", captured.Threads)
	}
	if captured.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s (from stack1), got %v", captured.Timeout)
	}
	if captured.Proxy != "http://proxy" {
		t.Errorf("expected proxy from stack2, got %s", captured.Proxy)
	}
}

func TestProfileStacking_CLIOverridesPrecedence(t *testing.T) {
	p1 := profile.Profile{
		Schema: 1,
		Name:   "scan/stack_cli",
		Tool:   "scan",
		Config: yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "threads"},
				{Kind: yaml.ScalarNode, Tag: "!!int", Value: "10"},
			},
		},
	}
	store := profile.NewStore()
	store.Create(p1)

	home, _ := os.UserHomeDir()
	defer os.RemoveAll(home + "/.config/searchit/profiles/scan/stack_cli.yaml")

	var captured config.Config
	cmd, opts := NewScanCmd()
	opts.URL = "http://localhost"
	opts.testHookConfigApplied = func(cfg config.Config) {
		captured = cfg
	}
	cmd.SetArgs([]string{"scan", "--profile", "scan/stack_cli", "--threads", "99"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("scan command failed: %v", err)
	}

	if captured.Threads != 99 {
		t.Errorf("expected CLI to override profile threads to 99, got %d", captured.Threads)
	}
}
