package resolver_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/resolver"
	"github.com/unsubble/searchit/internal/profile/types"
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

func TestResolver_Resolve(t *testing.T) {
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
		resolved, err := res.Resolve([]string{"scan/wordpress"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if len(resolved) != 2 {
			t.Fatalf("expected 2 profiles, got %d", len(resolved))
		}
		if resolved[0].Name != "scan/base" || resolved[1].Name != "scan/wordpress" {
			t.Errorf("expected order [base, wordpress], got [%s, %s]", resolved[0].Name, resolved[1].Name)
		}
	})

	t.Run("multiple dependencies", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/wordpress": {
					Name:    "scan/wordpress",
					Tool:    "scan",
					Depends: []string{"base", "php"},
				},
				"scan/base": {
					Name: "scan/base",
					Tool: "scan",
				},
				"scan/php": {
					Name: "scan/php",
					Tool: "scan",
				},
			},
		}

		res := resolver.New(store)
		resolved, err := res.Resolve([]string{"scan/wordpress"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if len(resolved) != 3 {
			t.Fatalf("expected 3 profiles, got %d", len(resolved))
		}
		if resolved[0].Name != "scan/base" || resolved[1].Name != "scan/php" || resolved[2].Name != "scan/wordpress" {
			t.Errorf("expected order [base, php, wordpress], got [%s, %s, %s]", resolved[0].Name, resolved[1].Name, resolved[2].Name)
		}
	})

	t.Run("nested dependencies", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/wordpress": {
					Name:    "scan/wordpress",
					Tool:    "scan",
					Depends: []string{"php"},
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
		resolved, err := res.Resolve([]string{"scan/wordpress"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if len(resolved) != 3 {
			t.Fatalf("expected 3 profiles, got %d", len(resolved))
		}
		if resolved[0].Name != "scan/base" || resolved[1].Name != "scan/php" || resolved[2].Name != "scan/wordpress" {
			t.Errorf("expected order [base, php, wordpress], got [%s, %s, %s]", resolved[0].Name, resolved[1].Name, resolved[2].Name)
		}
	})

	t.Run("duplicate dependency elimination", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/wordpress": {
					Name:    "scan/wordpress",
					Tool:    "scan",
					Depends: []string{"php"},
				},
				"scan/cms": {
					Name:    "scan/cms",
					Tool:    "scan",
					Depends: []string{"php"},
				},
				"scan/php": {
					Name: "scan/php",
					Tool: "scan",
				},
			},
		}

		res := resolver.New(store)
		resolved, err := res.Resolve([]string{"scan/wordpress", "scan/cms"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if len(resolved) != 3 {
			t.Fatalf("expected 3 profiles, got %d", len(resolved))
		}
		if resolved[0].Name != "scan/php" || resolved[1].Name != "scan/wordpress" || resolved[2].Name != "scan/cms" {
			t.Errorf("expected order [php, wordpress, cms], got [%s, %s, %s]", resolved[0].Name, resolved[1].Name, resolved[2].Name)
		}
	})

	t.Run("cycle detection", func(t *testing.T) {
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
					Depends: []string{"c"},
				},
				"scan/c": {
					Name:    "scan/c",
					Tool:    "scan",
					Depends: []string{"a"},
				},
			},
		}

		res := resolver.New(store)
		_, err := res.Resolve([]string{"scan/a"})
		if err == nil {
			t.Fatal("expected cyclic dependency error, got nil")
		}

		if !strings.Contains(err.Error(), "cyclic profile dependency detected") {
			t.Errorf("expected error to mention 'cyclic profile dependency detected', got: %v", err)
		}
		if !strings.Contains(err.Error(), "scan/a -> scan/b -> scan/c -> scan/a") {
			t.Errorf("expected chain scan/a -> scan/b -> scan/c -> scan/a in error, got: %v", err)
		}
	})

	t.Run("missing dependency", func(t *testing.T) {
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
		_, err := res.Resolve([]string{"scan/wordpress"})
		if err == nil {
			t.Fatal("expected error for missing dependency, got nil")
		}
		if !strings.Contains(err.Error(), "profile not found: scan/nonexistent") {
			t.Errorf("expected profile not found error, got: %v", err)
		}
	})

	t.Run("dependency ordering", func(t *testing.T) {
		store := &mockStore{
			profiles: map[string]*types.Profile{
				"scan/a": {
					Name:    "scan/a",
					Tool:    "scan",
					Depends: []string{"b", "c"},
				},
				"scan/b": {
					Name:    "scan/b",
					Tool:    "scan",
					Depends: []string{"d"},
				},
				"scan/c": {
					Name:    "scan/c",
					Tool:    "scan",
					Depends: []string{"d"},
				},
				"scan/d": {
					Name: "scan/d",
					Tool: "scan",
				},
			},
		}

		res := resolver.New(store)
		resolved, err := res.Resolve([]string{"scan/a"})
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}

		if len(resolved) != 4 {
			t.Fatalf("expected 4 profiles, got %d", len(resolved))
		}
		if resolved[0].Name != "scan/d" {
			t.Errorf("expected first profile to be scan/d, got %s", resolved[0].Name)
		}
		if resolved[3].Name != "scan/a" {
			t.Errorf("expected last profile to be scan/a, got %s", resolved[3].Name)
		}
	})
}
