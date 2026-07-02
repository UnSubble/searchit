package config_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/recursion"
)

func TestDefault_Threads(t *testing.T) {
	if got := config.Default().Threads; got != 32 {
		t.Errorf("Threads = %d, want 32", got)
	}
}

func TestDefault_Timeout(t *testing.T) {
	if got := config.Default().Timeout; got != 10 {
		t.Errorf("Timeout = %d, want 10", got)
	}
}

func TestDefault_Recursive(t *testing.T) {
	cfg := config.Default()
	if cfg.Recursive {
		t.Error("Recursive = true, want false")
	}
	if cfg.MaxDepth != 3 {
		t.Errorf("MaxDepth = %d, want 3", cfg.MaxDepth)
	}
	if cfg.Strategy != recursion.BFS {
		t.Errorf("Strategy = %v, want BFS", cfg.Strategy)
	}
}

func TestDefault_ExcludeFilters(t *testing.T) {
	cfg := config.Default()

	if len(cfg.Status.Exclude) == 0 {
		t.Fatal("Status.Exclude is empty, want at least one filter")
	}

	tests := []struct {
		code int
		want bool
	}{
		{404, true},
		{200, false},
		{500, false},
	}

	for _, tc := range tests {
		if got := cfg.Status.Exclude.Match(tc.code); got != tc.want {
			t.Errorf("Exclude.Match(%d) = %v, want %v", tc.code, got, tc.want)
		}
	}
}

func TestDefault_IncludeFiltersEmpty(t *testing.T) {
	if got := config.Default().Status.Include; len(got) != 0 {
		t.Errorf("Status.Include = %v, want empty", got)
	}
}

func TestDefault_RecurseOn(t *testing.T) {
	cfg := config.Default()
	if len(cfg.RecurseOn) == 0 {
		t.Fatal("RecurseOn is empty, want default filters")
	}

	tests := []struct {
		code int
		want bool
	}{
		{200, true},
		{301, true},
		{302, true},
		{403, true},
		{404, false},
		{500, false},
	}

	for _, tc := range tests {
		if got := cfg.RecurseOn.Match(tc.code); got != tc.want {
			t.Errorf("RecurseOn.Match(%d) = %v, want %v", tc.code, got, tc.want)
		}
	}
}
