package adaptive_test

import (
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/adaptive"
)

func TestLookup_ValidExact(t *testing.T) {
	p, ok := adaptive.Lookup("laravel")
	if !ok {
		t.Fatal("Lookup(\"laravel\") returned false, want true")
	}
	if p.ID != "laravel" {
		t.Errorf("ID = %q, want %q", p.ID, "laravel")
	}
	if p.DisplayName != "Laravel" {
		t.Errorf("DisplayName = %q, want %q", p.DisplayName, "Laravel")
	}
}

func TestLookup_CaseInsensitive(t *testing.T) {
	cases := []string{"LARAVEL", "Laravel", "lArAvEl", "laravel"}
	for _, input := range cases {
		p, ok := adaptive.Lookup(input)
		if !ok {
			t.Errorf("Lookup(%q) returned false, want true", input)
			continue
		}
		if p.ID != "laravel" {
			t.Errorf("Lookup(%q).ID = %q, want %q", input, p.ID, "laravel")
		}
	}
}

func TestLookup_WhitespaceTrimmed(t *testing.T) {
	p, ok := adaptive.Lookup("  spring  ")
	if !ok {
		t.Fatal("Lookup(\"  spring  \") returned false, want true")
	}
	if p.ID != "spring" {
		t.Errorf("ID = %q, want %q", p.ID, "spring")
	}
}

func TestLookup_Unknown(t *testing.T) {
	cases := []string{"rails", "symfony", "unknown", "", "next.js"}
	for _, input := range cases {
		_, ok := adaptive.Lookup(input)
		if ok {
			t.Errorf("Lookup(%q) returned true, want false", input)
		}
	}
}

func TestAll_ContainsBuiltins(t *testing.T) {
	all := adaptive.All()
	if len(all) == 0 {
		t.Fatal("All() returned empty slice")
	}

	// Build an ID set for assertion.
	ids := make(map[string]bool, len(all))
	for _, p := range all {
		ids[p.ID] = true
		if p.DisplayName == "" {
			t.Errorf("profile %q has empty DisplayName", p.ID)
		}
	}

	required := []string{
		"laravel", "wordpress", "spring", "django", "flask",
		"express", "nextjs", "nuxt", "aspnet", "react",
		"vue", "angular", "go",
	}
	for _, id := range required {
		if !ids[id] {
			t.Errorf("All() missing required technology %q", id)
		}
	}
}

func TestAll_Sorted(t *testing.T) {
	all := adaptive.All()
	for i := 1; i < len(all); i++ {
		if all[i].ID < all[i-1].ID {
			t.Errorf("All() not sorted: %q before %q", all[i-1].ID, all[i].ID)
		}
	}
}

func TestAll_IDsMatchLookup(t *testing.T) {
	for _, p := range adaptive.All() {
		got, ok := adaptive.Lookup(p.ID)
		if !ok {
			t.Errorf("Lookup(%q) failed but ID appears in All()", p.ID)
			continue
		}
		if got.ID != p.ID || got.DisplayName != p.DisplayName {
			t.Errorf("Lookup(%q) = %+v, want %+v", p.ID, got, p)
		}
	}
}

func TestSupportedIDs_ContainsAll(t *testing.T) {
	s := adaptive.SupportedIDs()
	for _, p := range adaptive.All() {
		if !strings.Contains(s, p.ID) {
			t.Errorf("SupportedIDs() missing %q", p.ID)
		}
	}
}
