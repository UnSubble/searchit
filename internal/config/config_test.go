package config_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/config"
)

func TestDefault_Threads(t *testing.T) {
	if got := config.Default().Threads; got != 64 {
		t.Errorf("Threads = %d, want 64", got)
	}
}

func TestDefault_Timeout(t *testing.T) {
	if got := config.Default().Timeout; got != 10 {
		t.Errorf("Timeout = %d, want 10", got)
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
