package profile

import (
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"
)

// Validator is the interface that tool-specific validators must implement.
type Validator interface {
	Tool() string
	Validate(p *Profile) error
}

var (
	validatorsMu sync.RWMutex
	validators   = make(map[string]Validator)
)

// Register registers a validator for a specific tool.
// Returns an error if a validator for the same tool is already registered.
func Register(v Validator) error {
	validatorsMu.Lock()
	defer validatorsMu.Unlock()
	tool := v.Tool()
	if _, ok := validators[tool]; ok {
		return fmt.Errorf("validator for tool %q is already registered", tool)
	}
	validators[tool] = v
	return nil
}

// GetValidator returns the registered validator for the tool, or nil if none.
func GetValidator(tool string) Validator {
	validatorsMu.RLock()
	defer validatorsMu.RUnlock()
	return validators[tool]
}

// Validate performs generic (tool-agnostic) validation on a profile.
func Validate(p *Profile) error {
	// - schema exists
	if p.Schema == 0 {
		return fmt.Errorf("profile schema version is missing or 0")
	}
	// - schema supported
	if p.Schema != 1 {
		return fmt.Errorf("unsupported profile schema version: %d (only schema version 1 is supported)", p.Schema)
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
