package fuzz

import (
	"fmt"

	"github.com/unsubble/searchit/internal/profile/scan"
	"github.com/unsubble/searchit/internal/profile/types"
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
func (v *FuzzValidator) Validate(p *types.Profile) error {
	var o scan.Overlay
	if err := p.Decode(&o); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}

	// Basic overlay validation matching CLI rules
	if o.Threads != nil && *o.Threads < 1 {
		return fmt.Errorf("threads must be at least 1")
	}
	if o.Rate != nil && *o.Rate <= 0 {
		return fmt.Errorf("rate must be greater than 0")
	}

	return nil
}
