package profile_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/resolver"
)

func TestBuiltinProfiles(t *testing.T) {
	// Register validators and decoders
	_ = profile.RegisterBuiltinValidators()
	_ = profile.RegisterBuiltinDecoders()

	store := profile.NewStore()

	// List of all expected built-in profile names
	expectedBuiltins := []string{
		"scan/base",
		"scan/default",
		"scan/quick",
		"scan/deep",
		"scan/paranoid",
		"scan/maniac",
		"scan/lightspeed",
		"scan-extra/api",
		"scan-extra/django",
		"scan-extra/dotnet",
		"scan-extra/express",
		"scan-extra/graphql",
		"scan-extra/java",
		"scan-extra/laravel",
		"scan-extra/nextjs",
		"scan-extra/node",
		"scan-extra/php",
		"scan-extra/rails",
		"scan-extra/spring",
		"scan-extra/static",
		"scan-extra/symfony",
		"scan-extra/wordpress",
	}

	for _, name := range expectedBuiltins {
		t.Run("load and validate "+name, func(t *testing.T) {
			p, err := store.Load(name)
			if err != nil {
				t.Fatalf("failed to load profile %s: %v", name, err)
			}

			// Validate schema exists and is supported, name matches, tool matches
			if err := profile.Validate(p); err != nil {
				t.Errorf("profile generic validation failed: %v", err)
			}

			// Validate tool-specific options (e.g. threads, max-depth, etc.)
			v := profile.GetValidator(p.Tool)
			if v == nil {
				t.Fatalf("no validator found for tool: %s", p.Tool)
			}
			if err := v.Validate(p); err != nil {
				t.Errorf("profile tool-specific validation failed: %v", err)
			}

			// Verify metadata is fully populated
			if p.Description == "" {
				t.Errorf("description is empty")
			}
			if p.Author == "" {
				t.Errorf("author is empty")
			}
			if p.License == "" {
				t.Errorf("license is empty")
			}
			if p.Homepage == "" {
				t.Errorf("homepage is empty")
			}
			if len(p.Tags) == 0 {
				t.Errorf("tags slice is empty")
			}
			if p.Experimental {
				t.Errorf("experimental should default to false")
			}
		})
	}

	t.Run("resolve dependency graphs and ensure no cycles exist", func(t *testing.T) {
		res := resolver.New(store)

		// Topologically resolve all profiles at once
		resolved, err := res.Resolve(expectedBuiltins)
		if err != nil {
			t.Fatalf("failed to resolve dependency graph: %v", err)
		}

		if len(resolved) != len(expectedBuiltins) {
			t.Errorf("expected %d resolved profiles, got %d", len(expectedBuiltins), len(resolved))
		}

		// Ensure order matches topological dependencies: quick must be after base, deep must be after quick
		baseIdx, quickIdx, deepIdx := -1, -1, -1
		for idx, p := range resolved {
			switch p.Name {
			case "scan/base":
				baseIdx = idx
			case "scan/quick":
				quickIdx = idx
			case "scan/deep":
				deepIdx = idx
			}
		}

		if baseIdx > quickIdx {
			t.Errorf("scan/base (idx %d) must appear before scan/quick (idx %d)", baseIdx, quickIdx)
		}
		if quickIdx > deepIdx {
			t.Errorf("scan/quick (idx %d) must appear before scan/deep (idx %d)", quickIdx, deepIdx)
		}
	})
}
