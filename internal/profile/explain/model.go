package explain

import (
	"github.com/unsubble/searchit/internal/profile/types"
)

// FieldOverride holds the value declared by a specific profile.
type FieldOverride struct {
	ProfileName string // e.g. "base", "php", "wordpress"
	Value       string // e.g. "32", "64", "128"
}

// FieldExplanation holds the final value and the historical overrides of a field.
type FieldExplanation struct {
	Name      string
	Final     string
	Overrides []FieldOverride
}

// ExplanationModel represents the full explanation details of a profile.
type ExplanationModel struct {
	TargetProfile  *types.Profile
	DependsChain   []string            // Full names of dependencies (topologically sorted)
	FinalConfig    map[string]string   // Single-value fields
	ListFields     map[string][]string // List-like fields ("Recurse On", "Exclude Status", "Include Headers", "Exclude Headers")
	FieldOverrides []FieldExplanation  // Fields that were explicitly overridden in the chain
}
