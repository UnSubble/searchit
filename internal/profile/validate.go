package profile

import (
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"
)

// Validator is the interface that tool-specific validators must implement.
type Validator interface {
	Validate(p *Profile) error
}

var (
	validatorsMu sync.RWMutex
	validators   = make(map[string]Validator)
)

// RegisterValidator registers a validator for a specific tool.
func RegisterValidator(tool string, v Validator) {
	validatorsMu.Lock()
	defer validatorsMu.Unlock()
	validators[tool] = v
}

// GetValidator returns the registered validator for the tool, or nil if none.
func GetValidator(tool string) Validator {
	validatorsMu.RLock()
	defer validatorsMu.RUnlock()
	return validators[tool]
}

// Validate performs generic (tool-agnostic) validation on a profile.
func Validate(p *Profile) error {
	// - version exists
	if p.Version == 0 {
		return fmt.Errorf("profile version is missing or 0")
	}
	// - version supported
	if p.Version != 1 {
		return fmt.Errorf("unsupported profile version: %d (only version 1 is supported)", p.Version)
	}
	// - tool exists
	if p.Tool == "" {
		return fmt.Errorf("profile tool is missing")
	}
	// - name exists
	if p.Name == "" {
		return fmt.Errorf("profile name is missing")
	}
	// - name is valid and namespace matches tool
	parsed, err := ParseName(p.Name)
	if err != nil {
		return fmt.Errorf("invalid profile name: %w", err)
	}
	if parsed.Tool != p.Tool {
		return fmt.Errorf("profile namespace/tool mismatch: name namespace is %q but tool is %q", parsed.Tool, p.Tool)
	}
	// - config node exists
	if p.Config.Kind == 0 {
		return fmt.Errorf("profile config section is missing")
	}
	// - config is a mapping
	if p.Config.Kind != yaml.MappingNode {
		return fmt.Errorf("profile config must be a YAML mapping")
	}

	return nil
}
