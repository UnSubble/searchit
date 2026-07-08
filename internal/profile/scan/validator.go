package scan

import (
	"fmt"

	"github.com/unsubble/searchit/internal/profile/types"
)

// ScanValidator implements profile.Validator for the scan tool.
type ScanValidator struct{}

// NewValidator returns a new instance of ScanValidator.
func NewValidator() *ScanValidator {
	return &ScanValidator{}
}

// Tool returns the tool name this validator handles.
func (v *ScanValidator) Tool() string {
	return "scan"
}

// Validate verifies that the profile configuration matches scan overlays.
func (v *ScanValidator) Validate(p *types.Profile) error {
	var o Overlay
	if err := p.Decode(&o); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}

	// Basic overlay validation matching CLI rules
	if o.Threads != nil && *o.Threads < 1 {
		return fmt.Errorf("threads must be at least 1")
	}
	if o.MaxDepth != nil && *o.MaxDepth < 1 {
		return fmt.Errorf("max-depth must be at least 1")
	}
	if o.Strategy != nil && *o.Strategy != "bfs" && *o.Strategy != "dfs" {
		return fmt.Errorf("invalid strategy %q: must be bfs or dfs", *o.Strategy)
	}
	if o.Rate != nil && *o.Rate <= 0 {
		return fmt.Errorf("rate must be greater than 0")
	}

	return nil
}
