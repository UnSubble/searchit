package resolver_test

import (
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/profile/resolver"
	"github.com/unsubble/searchit/internal/profile/types"
)

func TestResolver_BuildTree(t *testing.T) {
	t.Run("single dependency", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/wordpress": {
					Name:    "scan/wordpress",
					Tool:    "scan",
					Depends: []string{"base"},
				},
				"scan/base": {
					Name: "scan/base",
					Tool: "scan",
				},
			},
		}

		res := resolver.New(store)
		node, err := res.BuildTree("scan/wordpress")
		if err != nil {
			t.Fatalf("BuildTree failed: %v", err)
		}

		if node.Profile.Name != "scan/wordpress" {
			t.Errorf("expected root name scan/wordpress, got %s", node.Profile.Name)
		}
		if len(node.Children) != 1 {
			t.Fatalf("expected 1 child, got %d", len(node.Children))
		}
		if node.Children[0].Profile.Name != "scan/base" {
			t.Errorf("expected child name scan/base, got %s", node.Children[0].Profile.Name)
		}
	})

	t.Run("nested dependencies", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/wordpress": {
					Name:    "scan/wordpress",
					Tool:    "scan",
					Depends: []string{"scan/php"},
				},
				"scan/php": {
					Name:    "scan/php",
					Tool:    "scan",
					Depends: []string{"base"},
				},
				"scan/base": {
					Name: "scan/base",
					Tool: "scan",
				},
			},
		}

		res := resolver.New(store)
		node, err := res.BuildTree("scan/wordpress")
		if err != nil {
			t.Fatalf("BuildTree failed: %v", err)
		}

		// Verify depth: wordpress -> php -> base
		if len(node.Children) != 1 || node.Children[0].Profile.Name != "scan/php" {
			t.Fatalf("expected child scan/php")
		}
		phpNode := node.Children[0]
		if len(phpNode.Children) != 1 || phpNode.Children[0].Profile.Name != "scan/base" {
			t.Fatalf("expected grand-child scan/base")
		}
	})

	t.Run("multiple dependencies and order", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/wordpress": {
					Name:    "scan/wordpress",
					Tool:    "scan",
					Depends: []string{"base", "scan/api"},
				},
				"scan/base": {
					Name: "scan/base",
					Tool: "scan",
				},
				"scan/api": {
					Name: "scan/api",
					Tool: "scan",
				},
			},
		}

		res := resolver.New(store)
		node, err := res.BuildTree("scan/wordpress")
		if err != nil {
			t.Fatalf("BuildTree failed: %v", err)
		}

		// Verify order is preserved (base then scan/api)
		if len(node.Children) != 2 {
			t.Fatalf("expected 2 children, got %d", len(node.Children))
		}
		if node.Children[0].Profile.Name != "scan/base" {
			t.Errorf("expected first child scan/base, got %s", node.Children[0].Profile.Name)
		}
		if node.Children[1].Profile.Name != "scan/api" {
			t.Errorf("expected second child scan/api, got %s", node.Children[1].Profile.Name)
		}
	})

	t.Run("namespace inheritance", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/wordpress": {
					Name:    "scan/wordpress",
					Tool:    "scan",
					Depends: []string{"base"},
				},
				"scan/base": {
					Name: "scan/base",
					Tool: "scan",
				},
			},
		}

		res := resolver.New(store)
		node, err := res.BuildTree("scan/wordpress")
		if err != nil {
			t.Fatalf("BuildTree failed: %v", err)
		}

		// "base" inside "scan/wordpress" inherits "scan" namespace -> "scan/base"
		if node.Children[0].Profile.Name != "scan/base" {
			t.Errorf("expected inherited name scan/base, got %s", node.Children[0].Profile.Name)
		}
	})

	t.Run("cycle error", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/a": {
					Name:    "scan/a",
					Tool:    "scan",
					Depends: []string{"b"},
				},
				"scan/b": {
					Name:    "scan/b",
					Tool:    "scan",
					Depends: []string{"a"},
				},
			},
		}

		res := resolver.New(store)
		_, err := res.BuildTree("scan/a")
		if err == nil {
			t.Fatal("expected cycle error, got nil")
		}
		if !strings.Contains(err.Error(), "cyclic profile dependency detected") {
			t.Errorf("expected cycle error message, got: %v", err)
		}
	})

	t.Run("missing profile error", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/wordpress": {
					Name:    "scan/wordpress",
					Tool:    "scan",
					Depends: []string{"nonexistent"},
				},
			},
		}

		res := resolver.New(store)
		_, err := res.BuildTree("scan/wordpress")
		if err == nil {
			t.Fatal("expected error for missing profile, got nil")
		}
	})

	t.Run("rendering output", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/wordpress": {
					Name:    "scan/wordpress",
					Tool:    "scan",
					Depends: []string{"scan/php", "scan/api"},
				},
				"scan/php": {
					Name:    "scan/php",
					Tool:    "scan",
					Depends: []string{"base"},
				},
				"scan/base": {
					Name: "scan/base",
					Tool: "scan",
				},
				"scan/api": {
					Name: "scan/api",
					Tool: "scan",
				},
			},
		}

		res := resolver.New(store)
		node, err := res.BuildTree("scan/wordpress")
		if err != nil {
			t.Fatalf("BuildTree failed: %v", err)
		}

		gotOut := resolver.RenderTree(node)
		wantOut := "scan/wordpress\n├── scan/php\n│   └── scan/base\n└── scan/api"

		if gotOut != wantOut {
			t.Errorf("expected tree:\n%s\ngot:\n%s", wantOut, gotOut)
		}
	})
}
