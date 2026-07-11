package explain_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/explain"
	"github.com/unsubble/searchit/internal/profile/types"
	"gopkg.in/yaml.v3"
)

type mockStore struct {
	profiles map[string]*types.Profile
}

func (m *mockStore) Load(name string) (*types.Profile, error) {
	p, ok := m.profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile not found: %s", name)
	}
	return p, nil
}

func (m *mockStore) List() ([]profile.ProfileInfo, error) {
	return nil, nil
}

func (m *mockStore) LoadRaw(name string) ([]byte, error) {
	return nil, nil
}

func (m *mockStore) Create(p types.Profile) error {
	return nil
}

func parseYAMLNode(yamlStr string) yaml.Node {
	var node yaml.Node
	_ = yaml.Unmarshal([]byte(yamlStr), &node)
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return *node.Content[0]
	}
	return node
}

func TestExplain_BuildAndRender(t *testing.T) {
	// Register validator and decoder to ensure explain workflow runs successfully
	_ = profile.RegisterBuiltinValidators()
	_ = profile.RegisterBuiltinDecoders()

	t.Run("single profile without dependencies", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/single": {
					Schema:      1,
					Name:        "scan/single",
					Tool:        "scan",
					Description: "Just a single profile.",
					Author:      "TestAuthor",
					Tags:        []string{"test", "single"},
					Config:      parseYAMLNode("threads: 40\nrecursive: true"),
				},
			},
		}

		model, err := explain.Build(store, "scan/single")
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		if model.TargetProfile.Name != "scan/single" {
			t.Errorf("expected target scan/single, got %s", model.TargetProfile.Name)
		}
		if len(model.DependsChain) != 1 || model.DependsChain[0] != "scan/single" {
			t.Errorf("expected depends chain [scan/single], got %v", model.DependsChain)
		}

		// Threads overridden from default 32 to 40, Recursive from false to true
		if len(model.FieldOverrides) != 2 {
			t.Fatalf("expected 2 overrides, got %d", len(model.FieldOverrides))
		}

		rendered := explain.RenderText(model)

		// Check metadata rendering
		if !strings.Contains(rendered, "Description\n\n    Just a single profile.") {
			t.Errorf("missing description block, got:\n%s", rendered)
		}
		if !strings.Contains(rendered, "Author\n\n    TestAuthor") {
			t.Errorf("missing author block, got:\n%s", rendered)
		}
		if !strings.Contains(rendered, "Tags\n\n    test\n    single") {
			t.Errorf("missing tags block, got:\n%s", rendered)
		}

		// Check Final Configuration values
		if !strings.Contains(rendered, "Threads:              40") {
			t.Errorf("missing Threads 40 in Final Config, got:\n%s", rendered)
		}
		if !strings.Contains(rendered, "Recursive:            true") {
			t.Errorf("missing Recursive true in Final Config, got:\n%s", rendered)
		}

		// Check overrides rendering
		if !strings.Contains(rendered, "Threads\n\n    single        40\n\nFinal\n\n    40") {
			t.Errorf("missing Threads override list, got:\n%s", rendered)
		}
	})

	t.Run("multiple dependencies and override hierarchy", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/wordpress": {
					Schema:  1,
					Name:    "scan/wordpress",
					Tool:    "scan",
					Depends: []string{"scan/php"},
					Config:  parseYAMLNode("threads: 128\nrecursive: true"),
				},
				"scan/php": {
					Schema:  1,
					Name:    "scan/php",
					Tool:    "scan",
					Depends: []string{"base"},
					Config:  parseYAMLNode("threads: 64\nexclude-status: \"404,500\""),
				},
				"scan/base": {
					Schema: 1,
					Name:   "scan/base",
					Tool:   "scan",
					Config: parseYAMLNode("threads: 32"),
				},
			},
		}

		model, err := explain.Build(store, "scan/wordpress")
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		// Verify order resolved topologically
		wantChain := []string{"scan/base", "scan/php", "scan/wordpress"}
		for i, name := range model.DependsChain {
			if name != wantChain[i] {
				t.Errorf("chain at index %d = %s, want %s", i, name, wantChain[i])
			}
		}

		rendered := explain.RenderText(model)

		// Check dependency chain arrows
		wantArrows := "scan/base\n        \u2193\n    scan/php\n        \u2193\n    scan/wordpress"
		if !strings.Contains(rendered, wantArrows) {
			t.Errorf("missing dependency chain arrows, got:\n%s", rendered)
		}

		// Check override trace logs
		wantOverrides := "Threads\n\n    base          32\n\n    php           64\n\n    wordpress     128\n\nFinal\n\n    128"
		if !strings.Contains(rendered, wantOverrides) {
			t.Errorf("missing Threads overrides, got:\n%s", rendered)
		}
	})

	t.Run("missing profile error", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{},
		}
		_, err := explain.Build(store, "scan/wordpress")
		if err == nil {
			t.Fatal("expected missing profile error, got nil")
		}
	})

	t.Run("validation error propagation", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/invalid": {
					Schema: 2, // Unsupported schema
					Name:   "scan/invalid",
					Tool:   "scan",
					Config: parseYAMLNode("threads: 10"),
				},
			},
		}
		_, err := explain.Build(store, "scan/invalid")
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported profile schema version") {
			t.Errorf("expected schema validation error, got: %v", err)
		}
	})

	t.Run("tool-specific validation failure", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/invalid-threads": {
					Schema: 1,
					Name:   "scan/invalid-threads",
					Tool:   "scan",
					Config: parseYAMLNode("threads: 0"),
				},
			},
		}
		_, err := explain.Build(store, "scan/invalid-threads")
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
		if !strings.Contains(err.Error(), "threads must be at least 1") {
			t.Errorf("expected threads validation error, got: %v", err)
		}
	})

	t.Run("decode overlay failure", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/invalid-format": {
					Schema: 1,
					Name:   "scan/invalid-format",
					Tool:   "scan",
					Config: parseYAMLNode("threads: abc"),
				},
			},
		}
		_, err := explain.Build(store, "scan/invalid-format")
		if err == nil {
			t.Fatal("expected decode error, got nil")
		}
	})

	t.Run("all metadata fields and configuration options", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/complex": {
					Schema:       1,
					Name:         "scan/complex",
					Tool:         "scan",
					Description:  "Complex configuration description",
					Author:       "Author Name",
					Homepage:     "http://example.com",
					License:      "MIT",
					Experimental: true,
					Created:      "2026-01-01",
					Updated:      "2026-01-02",
					Config: parseYAMLNode(`
threads: 50
recursive: true
strategy: dfs
max-depth: 5
output: ndjson
timeout: 15s
delay: 150ms
rate: 2.5
recurse-on: "200,301"
exclude-status: "404"
include-headers: ["Server=nginx"]
exclude-headers: ["Server=Apache"]
`),
				},
			},
		}

		model, err := explain.Build(store, "scan/complex")
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		rendered := explain.RenderText(model)

		wantSubstrings := []string{
			"Profile\n\n    scan/complex",
			"Description\n\n    Complex configuration description",
			"Author\n\n    Author Name",
			"Homepage\n\n    http://example.com",
			"License\n\n    MIT",
			"Experimental\n\n    true",
			"Created\n\n    2026-01-01",
			"Updated\n\n    2026-01-02",
			"Threads:              50",
			"Recursive:            true",
			"Strategy:             dfs",
			"Max Depth:            5",
			"Output:               ndjson",
			"Timeout:              15s",
			"Delay:                150ms",
			"Rate:                 2.5",
			"Recurse On:\n\n    200\n    301",
			"Exclude Status:\n\n    404",
			"Include Headers:\n\n    Server=nginx",
			"Exclude Headers:\n\n    Server=Apache",
		}

		for _, s := range wantSubstrings {
			if !strings.Contains(rendered, s) {
				t.Errorf("expected output to contain %q, but did not. Got:\n%s", s, rendered)
			}
		}
	})
}
