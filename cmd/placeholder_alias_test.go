package cmd

import (
	"context"
	"net/http"
	"testing"

	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/wordlist"
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

// TestResolveSecondaryWordlist_EmbeddedFUZZ_Regression covers the bug where aliasing
// =fuzz when FUZZ uses the default embedded wordlist (opts.Wordlist == "") previously
// produced an empty slice []string{} and collapsed BUZZ substitutions to "".
func TestResolveSecondaryWordlist_EmbeddedFUZZ_Regression(t *testing.T) {
	ctx := context.Background()

	// Load effective FUZZ vocabulary (embedded)
	fuzzWords, err := wordlist.LoadEffectiveWords(ctx, "")
	if err != nil {
		t.Fatalf("failed to load embedded FUZZ vocabulary: %v", err)
	}
	if len(fuzzWords) != 4751 {
		t.Fatalf("expected 4751 embedded entries, got %d", len(fuzzWords))
	}

	loaded := map[string][]string{
		"FUZZ": fuzzWords,
	}

	for _, ph := range []string{"FOO", "BAR", "BAZ", "BUZZ"} {
		t.Run("alias "+ph+" to embedded fuzz", func(t *testing.T) {
			words, err := resolveSecondaryWordlist("=fuzz", ph, loaded)
			if err != nil {
				t.Fatalf("resolveSecondaryWordlist(=fuzz, %s) failed: %v", ph, err)
			}
			if len(words) != 4751 {
				t.Fatalf("expected 4751 words for %s, got %d", ph, len(words))
			}

			// Verify vocabulary elements are non-empty and match embedded list
			for i, w := range words {
				if w == "" {
					t.Fatalf("word[%d] in %s is empty string", i, ph)
				}
				if w != fuzzWords[i] {
					t.Fatalf("word[%d] mismatch: got %q, want %q", i, w, fuzzWords[i])
				}
			}

			// Verify slice independence (cloning)
			words[0] = "MODIFIED"
			if fuzzWords[0] == "MODIFIED" {
				t.Fatalf("mutation of %s affected original FUZZ vocabulary", ph)
			}
		})
	}
}

// TestResolveSecondaryWordlist_ChainedAliases verifies that multi-hop alias chains
// (=fuzz -> =buzz -> =bar -> =foo) resolve correctly with independent copies.
func TestResolveSecondaryWordlist_ChainedAliases(t *testing.T) {
	ctx := context.Background()
	fuzzWords, err := wordlist.LoadEffectiveWords(ctx, "")
	if err != nil {
		t.Fatalf("failed to load embedded FUZZ vocabulary: %v", err)
	}

	loaded := map[string][]string{
		"FUZZ": fuzzWords,
	}

	// BUZZ -> FUZZ
	buzzWords, err := resolveSecondaryWordlist("=fuzz", "BUZZ", loaded)
	if err != nil {
		t.Fatalf("BUZZ alias failed: %v", err)
	}
	loaded["BUZZ"] = buzzWords

	// BAR -> BUZZ
	barWords, err := resolveSecondaryWordlist("=buzz", "BAR", loaded)
	if err != nil {
		t.Fatalf("BAR alias failed: %v", err)
	}
	loaded["BAR"] = barWords

	// FOO -> BAR
	fooWords, err := resolveSecondaryWordlist("=bar", "FOO", loaded)
	if err != nil {
		t.Fatalf("FOO alias failed: %v", err)
	}
	loaded["FOO"] = fooWords

	// BAZ -> FOO
	bazWords, err := resolveSecondaryWordlist("=foo", "BAZ", loaded)
	if err != nil {
		t.Fatalf("BAZ alias failed: %v", err)
	}
	loaded["BAZ"] = bazWords

	if len(buzzWords) != 4751 || len(barWords) != 4751 || len(fooWords) != 4751 || len(bazWords) != 4751 {
		t.Errorf("expected all 4 chained aliases to have 4751 words; got buzz=%d, bar=%d, foo=%d, baz=%d",
			len(buzzWords), len(barWords), len(fooWords), len(bazWords))
	}
}

// TestCandidateEstimation_EmbeddedAlias verifies that TotalCandidates is calculated
// as 4751 * 4751 = 22572001 when aliasing the embedded FUZZ wordlist.
func TestCandidateEstimation_EmbeddedAlias(t *testing.T) {
	ctx := context.Background()
	fuzzWords, err := wordlist.LoadEffectiveWords(ctx, "")
	if err != nil {
		t.Fatalf("failed to load embedded FUZZ vocabulary: %v", err)
	}

	buzzWords := make([]string, len(fuzzWords))
	copy(buzzWords, fuzzWords)

	runner := &fuzz.Runner{
		TargetURL:       "https://FUZZ.futurevera.thm/",
		HeaderTemplates: http.Header{"Host": []string{"BUZZ.futurevera.thm"}},
		BuzzWords:       buzzWords,
	}

	total := runner.EstimateCandidates(len(fuzzWords))
	const expected = int64(4751) * int64(4751) // 22,572,001
	if total != expected {
		t.Errorf("EstimateCandidates = %d, want %d", total, expected)
	}
}

// TestGenerator_NoEmptyPlaceholderSubstitutions verifies that generator permutations
// always substitute valid non-empty vocabulary entries when using an alias to embedded FUZZ.
func TestGenerator_NoEmptyPlaceholderSubstitutions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fuzzWords := []string{"admin", "api"}
	buzzWords := []string{"admin", "api"}

	gen := fuzz.NewGenerator(
		"https://FUZZ.futurevera.thm/",
		"GET",
		"",
		http.Header{"Host": []string{"BUZZ.futurevera.thm"}},
		"",
		nil,
		nil,
		nil,
		buzzWords,
	)

	primaryChan := make(chan string, 10)
	for _, w := range fuzzWords {
		primaryChan <- w
	}
	close(primaryChan)

	jobsChan := make(chan fuzz.RequestDTO, 10)
	go func() {
		defer close(jobsChan)
		gen.Generate(ctx, primaryChan, jobsChan)
	}()

	var generated []fuzz.RequestDTO
	for job := range jobsChan {
		generated = append(generated, job)
		host := http.Header(job.Headers).Get("Host")
		if host == ".futurevera.thm" || host == "" {
			t.Errorf("corrupt job generated with empty BUZZ substitution: URL=%s Host=%s", job.URL, host)
		}
	}

	if len(generated) != 4 { // 2 * 2 = 4
		t.Errorf("expected 4 Cartesian jobs, got %d", len(generated))
	}
}
