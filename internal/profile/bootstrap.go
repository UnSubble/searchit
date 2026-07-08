package profile

import (
	"github.com/unsubble/searchit/internal/profile/scan"
	v1 "github.com/unsubble/searchit/internal/profile/schema/v1"
)

// RegisterBuiltinValidators registers all built-in validators with the registry.
// This must be called explicitly during application startup.
func RegisterBuiltinValidators() error {
	if err := Register(scan.NewValidator()); err != nil {
		return err
	}
	return nil
}

// RegisterBuiltinDecoders registers all built-in schema decoders with the registry.
// This must be called explicitly during application startup.
func RegisterBuiltinDecoders() error {
	if err := RegisterDecoder(v1.New()); err != nil {
		return err
	}
	return nil
}
