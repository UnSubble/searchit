package scan_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/scan"
	"gopkg.in/yaml.v3"
)

func TestScanValidator_Validate(t *testing.T) {
	v := &scan.ScanValidator{}

	t.Run("valid config", func(t *testing.T) {
		var node yaml.Node
		_ = yaml.Unmarshal([]byte(`
threads: 10
max-depth: 3
strategy: bfs
rate: 1.5
`), &node)

		mappingNode := node.Content[0]

		p := &profile.Profile{
			Name:   "scan/test",
			Tool:   "scan",
			Config: *mappingNode,
		}

		if err := v.Validate(p); err != nil {
			t.Fatalf("expected valid config to pass, got: %v", err)
		}
	})

	t.Run("invalid threads", func(t *testing.T) {
		var node yaml.Node
		_ = yaml.Unmarshal([]byte(`threads: 0`), &node)
		mappingNode := node.Content[0]
		p := &profile.Profile{Config: *mappingNode}
		if err := v.Validate(p); err == nil {
			t.Fatal("expected validation to fail for threads < 1, got nil")
		}
	})

	t.Run("invalid max-depth", func(t *testing.T) {
		var node yaml.Node
		_ = yaml.Unmarshal([]byte(`max-depth: 0`), &node)
		mappingNode := node.Content[0]
		p := &profile.Profile{Config: *mappingNode}
		if err := v.Validate(p); err == nil {
			t.Fatal("expected validation to fail for max-depth < 1, got nil")
		}
	})

	t.Run("invalid strategy", func(t *testing.T) {
		var node yaml.Node
		_ = yaml.Unmarshal([]byte(`strategy: invalid`), &node)
		mappingNode := node.Content[0]
		p := &profile.Profile{Config: *mappingNode}
		if err := v.Validate(p); err == nil {
			t.Fatal("expected validation to fail for invalid strategy, got nil")
		}
	})

	t.Run("invalid rate", func(t *testing.T) {
		var node yaml.Node
		_ = yaml.Unmarshal([]byte(`rate: -5`), &node)
		mappingNode := node.Content[0]
		p := &profile.Profile{Config: *mappingNode}
		if err := v.Validate(p); err == nil {
			t.Fatal("expected validation to fail for rate <= 0, got nil")
		}
	})

	t.Run("decode error", func(t *testing.T) {
		var node yaml.Node
		_ = yaml.Unmarshal([]byte(`threads: "abc"`), &node)
		mappingNode := node.Content[0]
		p := &profile.Profile{Config: *mappingNode}
		if err := v.Validate(p); err == nil {
			t.Fatal("expected validation to fail for invalid threads format, got nil")
		}
	})

	t.Run("Tool method", func(t *testing.T) {
		if v.Tool() != "scan" {
			t.Errorf("expected Tool() to be scan, got %s", v.Tool())
		}
	})
}
