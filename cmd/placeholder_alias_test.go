package cmd

import (
	"testing"
)

// TestIsAlias verifies the isAlias helper.
func TestIsAlias(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"=fuzz", true},
		{"=FOO", true},
		{"=bar", true},
		{"", false},
		{"users.txt", false},
		{"./wordlist.txt", false},
		{"fuzz", false},
	}
	for _, tc := range cases {
		got := isAlias(tc.input)
		if got != tc.want {
			t.Errorf("isAlias(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// TestValidatePlaceholderAliases covers all validation rules.
func TestValidatePlaceholderAliases(t *testing.T) {
	t.Run("valid alias to fuzz", func(t *testing.T) {
		err := validatePlaceholderAliases(map[string]string{
			"FOO":  "=fuzz",
			"BAR":  "",
			"BAZ":  "",
			"BUZZ": "",
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid alias case-insensitive", func(t *testing.T) {
		err := validatePlaceholderAliases(map[string]string{
			"FOO":  "=FUZZ",
			"BAR":  "",
			"BAZ":  "",
			"BUZZ": "",
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("multiple valid aliases", func(t *testing.T) {
		err := validatePlaceholderAliases(map[string]string{
			"FOO":  "=fuzz",
			"BAR":  "=fuzz",
			"BAZ":  "=foo",
			"BUZZ": "",
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("no aliases (plain file paths)", func(t *testing.T) {
		err := validatePlaceholderAliases(map[string]string{
			"FOO":  "users.txt",
			"BAR":  "dirs.txt",
			"BAZ":  "",
			"BUZZ": "",
		})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("self alias rejected", func(t *testing.T) {
		err := validatePlaceholderAliases(map[string]string{
			"FOO":  "=foo",
			"BAR":  "",
			"BAZ":  "",
			"BUZZ": "",
		})
		if err == nil {
			t.Error("expected error for self-alias, got nil")
		}
	})

	t.Run("unknown target rejected", func(t *testing.T) {
		err := validatePlaceholderAliases(map[string]string{
			"FOO":  "=unknown",
			"BAR":  "",
			"BAZ":  "",
			"BUZZ": "",
		})
		if err == nil {
			t.Error("expected error for unknown alias target, got nil")
		}
	})

	t.Run("circular alias rejected foo<->bar", func(t *testing.T) {
		err := validatePlaceholderAliases(map[string]string{
			"FOO":  "=bar",
			"BAR":  "=foo",
			"BAZ":  "",
			"BUZZ": "",
		})
		if err == nil {
			t.Error("expected error for circular alias, got nil")
		}
	})

	t.Run("circular alias rejected buzz<->foo<->bar", func(t *testing.T) {
		err := validatePlaceholderAliases(map[string]string{
			"FOO":  "=bar",
			"BAR":  "=buzz",
			"BAZ":  "",
			"BUZZ": "=foo",
		})
		if err == nil {
			t.Error("expected error for circular alias chain, got nil")
		}
	})
}

// TestResolveSecondaryWordlist covers file loading and alias resolution.
func TestResolveSecondaryWordlist(t *testing.T) {
	t.Run("empty raw returns nil", func(t *testing.T) {
		words, err := resolveSecondaryWordlist("", "FOO", nil)
		if err != nil || words != nil {
			t.Errorf("got words=%v err=%v; want nil,nil", words, err)
		}
	})

	t.Run("alias to loaded FUZZ returns a copy", func(t *testing.T) {
		fuzzWords := []string{"admin", "login", "index"}
		loaded := map[string][]string{"FUZZ": fuzzWords}

		words, err := resolveSecondaryWordlist("=fuzz", "BUZZ", loaded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(words) != len(fuzzWords) {
			t.Errorf("expected %d words, got %d", len(fuzzWords), len(words))
		}
		for i, w := range fuzzWords {
			if words[i] != w {
				t.Errorf("word[%d] = %q; want %q", i, words[i], w)
			}
		}

		// Verify independence: mutating the returned slice must NOT affect fuzzWords.
		if len(words) > 0 {
			words[0] = "MUTATED"
			if fuzzWords[0] == "MUTATED" {
				t.Error("resolveSecondaryWordlist returned a reference to the original slice, not an independent copy")
			}
		}
	})

	t.Run("alias to loaded FUZZ case-insensitive", func(t *testing.T) {
		fuzzWords := []string{"a", "b"}
		loaded := map[string][]string{"FUZZ": fuzzWords}

		words, err := resolveSecondaryWordlist("=FUZZ", "FOO", loaded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(words) != 2 {
			t.Errorf("expected 2 words, got %d", len(words))
		}
	})

	t.Run("alias to empty loaded slice returns empty", func(t *testing.T) {
		loaded := map[string][]string{"FUZZ": {}}
		words, err := resolveSecondaryWordlist("=fuzz", "BUZZ", loaded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(words) != 0 {
			t.Errorf("expected 0 words, got %d", len(words))
		}
	})

	t.Run("alias to missing key returns error", func(t *testing.T) {
		loaded := map[string][]string{}
		_, err := resolveSecondaryWordlist("=fuzz", "BUZZ", loaded)
		if err == nil {
			t.Error("expected error for missing alias target, got nil")
		}
	})
}

// TestResolveSecondaryWordlist_CartesianIndependence verifies that two placeholders
// aliased to the same FUZZ wordlist maintain independent slice instances, enabling
// correct Cartesian product iteration (they must not share pointer state).
func TestResolveSecondaryWordlist_CartesianIndependence(t *testing.T) {
	fuzzWords := []string{"alpha", "beta", "gamma"}
	loaded := map[string][]string{"FUZZ": fuzzWords}

	fooWords, err1 := resolveSecondaryWordlist("=fuzz", "FOO", loaded)
	buzzWords, err2 := resolveSecondaryWordlist("=fuzz", "BUZZ", loaded)

	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v %v", err1, err2)
	}

	if len(fooWords) != 3 || len(buzzWords) != 3 {
		t.Errorf("expected 3 words each; got foo=%d buzz=%d", len(fooWords), len(buzzWords))
	}

	// Mutate one slice; the other must be unaffected.
	fooWords[0] = "MUTATED"
	if buzzWords[0] == "MUTATED" {
		t.Error("fooWords and buzzWords share the same backing array — Cartesian product state is not independent")
	}
	// Verify the original fuzz words are also untouched.
	if fuzzWords[0] == "MUTATED" {
		t.Error("fooWords mutation propagated to the original fuzzWords")
	}
}
