package types_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/profile/types"
	"gopkg.in/yaml.v3"
)

type dummyConfig struct {
	Threads int `yaml:"threads"`
}

func TestProfile_Decode(t *testing.T) {
	t.Run("Empty Config Node", func(t *testing.T) {
		p := &types.Profile{
			Name: "empty",
		}
		var cfg dummyConfig
		if err := p.Decode(&cfg); err != nil {
			t.Fatalf("expected nil error on empty config, got %v", err)
		}
		if cfg.Threads != 0 {
			t.Errorf("expected 0 threads, got %d", cfg.Threads)
		}
	})

	t.Run("Populated Config Node", func(t *testing.T) {
		var node yaml.Node
		if err := yaml.Unmarshal([]byte("threads: 64"), &node); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}

		p := &types.Profile{
			Name:   "custom",
			Config: node,
		}
		var cfg dummyConfig
		if err := p.Decode(&cfg); err != nil {
			t.Fatalf("unexpected decode error: %v", err)
		}
		if cfg.Threads != 64 {
			t.Errorf("expected 64 threads, got %d", cfg.Threads)
		}
	})
}
