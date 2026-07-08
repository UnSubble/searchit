package profile

import (
	"github.com/unsubble/searchit/internal/profile/scan"
)

// RegisterBuiltinValidators registers all built-in validators with the registry.
// This must be called explicitly during application startup.
func RegisterBuiltinValidators() error {
	if err := Register(scan.NewValidator()); err != nil {
		return err
	}
	return nil
}
