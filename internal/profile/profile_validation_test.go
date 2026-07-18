package profile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/resolver"
)

func TestProfileValidation_InheritanceAndMerge(t *testing.T) {
	tmpDir := t.TempDir()

	parentYaml := `
schema: 1
name: scan/parent
tool: scan
description: Parent profile
author: Test
license: MIT
homepage: http://test.com
tags: [test]
config:
  threads: 64
  timeout: 5
  recursive: true
  max-depth: 2
`
	childYaml := `
schema: 1
name: scan/child
tool: scan
description: Child profile inheriting parent
author: Test
license: MIT
homepage: http://test.com
tags: [test]
depends:
  - scan/parent
config:
  timeout: 15
  max-depth: 4
`
	// Since DefaultStore reads user profiles from s.UserDir using relative paths as names:
	// "scan/parent" matches "scan/parent.yaml" under UserDir.
	if err := os.MkdirAll(filepath.Join(tmpDir, "scan"), 0755); err != nil {
		t.Fatalf("failed to create scan dir: %v", err)
	}

	parentPath := filepath.Join(tmpDir, "scan", "parent.yaml")
	childPath := filepath.Join(tmpDir, "scan", "child.yaml")
	if err := os.WriteFile(parentPath, []byte(parentYaml), 0600); err != nil {
		t.Fatalf("failed to write parent: %v", err)
	}
	if err := os.WriteFile(childPath, []byte(childYaml), 0600); err != nil {
		t.Fatalf("failed to write child: %v", err)
	}

	store := profile.NewStore()
	store.UserDir = tmpDir

	// Resolve the child profile dependencies
	res := resolver.New(store)
	resolved, err := res.Resolve([]string{"scan/child"})
	if err != nil {
		t.Fatalf("failed to resolve child: %v", err)
	}

	if len(resolved) != 2 {
		t.Fatalf("expected 2 profiles resolved, got %d", len(resolved))
	}

	// Merge options from the chain manually to check values
	var finalConfig scanConfig
	for _, p := range resolved {
		if err := p.Decode(&finalConfig); err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
	}

	// Threads: inherited (64)
	if finalConfig.Threads != 64 {
		t.Errorf("expected Threads=64, got %d", finalConfig.Threads)
	}
	// Timeout: overridden (15)
	if finalConfig.Timeout != 15 {
		t.Errorf("expected Timeout=15, got %d", finalConfig.Timeout)
	}
	// Recursive: inherited (true)
	if !finalConfig.Recursive {
		t.Error("expected Recursive=true")
	}
	// MaxDepth: overridden (4)
	if finalConfig.MaxDepth != 4 {
		t.Errorf("expected MaxDepth=4, got %d", finalConfig.MaxDepth)
	}
}

func TestProfileValidation_InvalidConfigs(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		namespace string
		yaml      string
		wantErr   string
	}{
		{
			name:      "negative threads",
			namespace: "scan",
			yaml: `
schema: 1
name: scan/invalid
tool: scan
description: Invalid
author: Test
license: MIT
homepage: http://test.com
tags: [test]
config:
  threads: -5
`,
			wantErr: "threads",
		},
		{
			name:      "invalid tool name",
			namespace: "custom",
			yaml: `
schema: 1
name: custom/invalid
tool: unknown_tool
description: Invalid
author: Test
license: MIT
homepage: http://test.com
tags: [test]
config: {}
`,
			wantErr: "tool",
		},
		{
			name:      "missing name",
			namespace: "scan",
			yaml: `
schema: 1
tool: scan
description: Invalid
author: Test
license: MIT
homepage: http://test.com
tags: [test]
config: {}
`,
			wantErr: "name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dirPath := filepath.Join(tmpDir, tc.namespace)
			if err := os.MkdirAll(dirPath, 0755); err != nil {
				t.Fatalf("failed to create dir: %v", err)
			}

			// Under UserDir, the relative path corresponds to the profile name.
			// So if we load "scan/invalid", the file must be scan/invalid.yaml.
			// If we load "custom/invalid", the file must be custom/invalid.yaml.
			path := filepath.Join(dirPath, "invalid.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0600); err != nil {
				t.Fatalf("failed to write yaml: %v", err)
			}

			store := profile.NewStore()
			store.UserDir = tmpDir

			p, err := store.Load(tc.namespace + "/invalid")
			if err != nil {
				// Loading failed (e.g. missing name check during readUserProfiles)
				if tc.wantErr == "name" {
					return
				}
				t.Fatalf("failed to load invalid profile: %v", err)
			}

			// Validate using generic validators
			err1 := profile.Validate(p)
			var err2 error
			v := profile.GetValidator(p.Tool)
			if v != nil {
				err2 = v.Validate(p)
			}

			combinedErr := ""
			if err1 != nil {
				combinedErr += err1.Error()
			}
			if err2 != nil {
				combinedErr += " " + err2.Error()
			}

			if combinedErr == "" {
				t.Error("expected validation error, got none")
			} else if !strings.Contains(strings.ToLower(combinedErr), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %q", tc.wantErr, combinedErr)
			}
		})
	}
}
