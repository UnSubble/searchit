package useragent_test

import (
	"slices"
	"testing"

	"github.com/unsubble/searchit/internal/useragent"
)

// --------------------------------------------------------------------------
// Resolve – precedence tests
// --------------------------------------------------------------------------

func TestResolve_ExplicitHeaderWins(t *testing.T) {
	// -H "User-Agent=..." must beat every other source.
	got := useragent.Resolve("from-header", "from-flag", "from-random")
	if got != "from-header" {
		t.Errorf("expected header value to win, got %q", got)
	}
}

func TestResolve_ExplicitFlagWins(t *testing.T) {
	// --user-agent must beat profile and --random-agent.
	got := useragent.Resolve("", "from-flag", "from-random")
	if got != "from-flag" {
		t.Errorf("expected flag value to win, got %q", got)
	}
}

func TestResolve_RandomAgentUsed(t *testing.T) {
	// --random-agent or profile random-agent:true (treated identically) wins
	// when no explicit sources are set.
	got := useragent.Resolve("", "", "from-random")
	if got != "from-random" {
		t.Errorf("expected randomUA value, got %q", got)
	}
}

func TestResolve_NoConfig_ReturnsEmpty(t *testing.T) {
	// When nothing is configured the caller should not set a User-Agent header.
	got := useragent.Resolve("", "", "")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// --------------------------------------------------------------------------
// Random – correctness
// --------------------------------------------------------------------------

func TestRandom_ReturnsKnownAgent(t *testing.T) {
	// Random must always return a value from the embedded list.
	agents := useragent.Agents()
	for i := range 20 {
		got := useragent.Random()
		if !slices.Contains(agents, got) {
			t.Errorf("iteration %d: Random() returned %q which is not in the embedded list", i, got)
		}
	}
}
