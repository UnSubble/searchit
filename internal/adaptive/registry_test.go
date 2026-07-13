package adaptive_test

import (
	"strings"
	"sync"
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

func TestRegistry_Stress(t *testing.T) {
	// 1. Extreme lookup values: extremely long string, empty string
	longID := strings.Repeat("a", 10000)
	if _, ok := adaptive.Lookup(longID); ok {
		t.Error("expected Lookup of 10000 characters to fail")
	}

	// 2. High concurrency lookups
	const workers = 50
	const lookupsPerWorker = 500
	var startBarrier sync.WaitGroup
	var workersWg sync.WaitGroup

	startBarrier.Add(1)
	workersWg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(workerID int) {
			defer workersWg.Done()
			// Wait for start signal to maximize concurrency contention
			startBarrier.Wait()

			techs := []string{"laravel", "spring", "wordpress", "express", "LARAVEL", "Django", "  go  ", "invalid"}
			for j := 0; j < lookupsPerWorker; j++ {
				input := techs[j%len(techs)]
				p, ok := adaptive.Lookup(input)
				if input == "invalid" {
					if ok {
						t.Errorf("expected failure for %q", input)
					}
				} else {
					if !ok {
						t.Errorf("expected success for %q", input)
					}
					canonical := strings.ToLower(strings.TrimSpace(input))
					if p.ID != canonical {
						t.Errorf("got %q, want %q", p.ID, canonical)
					}
				}
			}
		}(i)
	}

	startBarrier.Done()
	workersWg.Wait()
}
