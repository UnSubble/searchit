package fuzz

import (
	"fmt"
	"net/url"
	"regexp"

	"github.com/unsubble/searchit/internal/profile/types"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
)

// FuzzValidator implements profile.Validator for the fuzz tool.
type FuzzValidator struct{}

// NewValidator returns a new instance of FuzzValidator.
func NewValidator() *FuzzValidator {
	return &FuzzValidator{}
}

// Tool returns the tool name this validator handles.
func (v *FuzzValidator) Tool() string {
	return "fuzz"
}

// Validate verifies that the profile configuration matches fuzz overlays.
// Rules mirror those enforced by the CLI PreRunE validation.
func (v *FuzzValidator) Validate(p *types.Profile) error {
	var o Overlay
	if err := p.Decode(&o); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}

	if o.Threads != nil && *o.Threads < 1 {
		return fmt.Errorf("threads must be at least 1")
	}
	if o.Rate != nil && *o.Rate <= 0 {
		return fmt.Errorf("rate must be greater than 0")
	}
	if o.Strategy != nil {
		s := *o.Strategy
		if s != "eager" && s != "bfs" && s != "dfs" && s != "priority" {
			return fmt.Errorf("invalid strategy %q: must be eager, bfs, dfs, or priority", s)
		}
	}
	if o.MaxRedirects != nil && *o.MaxRedirects < 0 {
		return fmt.Errorf("max-redirects cannot be negative")
	}

	if o.MatchStatus != nil && *o.MatchStatus != "" {
		if _, err := status.Parse(*o.MatchStatus); err != nil {
			return fmt.Errorf("invalid match-status: %w", err)
		}
	}
	if o.ExcludeStatus != nil && *o.ExcludeStatus != "" {
		if _, err := status.Parse(*o.ExcludeStatus); err != nil {
			return fmt.Errorf("invalid exclude-status: %w", err)
		}
	}
	if o.IncludeSize != nil && *o.IncludeSize != "" {
		if _, err := size.Parse(*o.IncludeSize); err != nil {
			return fmt.Errorf("invalid include-size: %w", err)
		}
	}
	if o.ExcludeSize != nil && *o.ExcludeSize != "" {
		if _, err := size.Parse(*o.ExcludeSize); err != nil {
			return fmt.Errorf("invalid exclude-size: %w", err)
		}
	}
	if o.MatchRegex != nil {
		for _, p := range *o.MatchRegex {
			if _, err := regexp.Compile(p); err != nil {
				return fmt.Errorf("invalid match-regex %q: %w", p, err)
			}
		}
	}
	if o.FilterRegex != nil {
		for _, p := range *o.FilterRegex {
			if _, err := regexp.Compile(p); err != nil {
				return fmt.Errorf("invalid filter-regex %q: %w", p, err)
			}
		}
	}
	if o.Proxy != nil && *o.Proxy != "" {
		if _, err := url.Parse(*o.Proxy); err != nil {
			return fmt.Errorf("invalid proxy URL: %w", err)
		}
	}
	if o.URL != nil && *o.URL != "" {
		if _, err := url.Parse(*o.URL); err != nil {
			return fmt.Errorf("invalid url: %w", err)
		}
	}

	return nil
}
