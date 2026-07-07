// Package profile implements Searchit's profile management system.
//
// Profiles are first-class, tool-agnostic resources. They store metadata
// (name, tool, description) alongside raw configuration as a yaml.Node.
// The profile package never interprets configuration — it delegates that
// to each tool via the Decode method.
//
// Profiles are resolved from user directories and embedded defaults,
// with user profiles taking precedence.
package profile

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Profile is the top-level profile document.
//
// Config is stored as a yaml.Node so the profile package remains
// completely agnostic to tool-specific configuration schemas. Each
// tool decodes Config into its own struct via [Profile.Decode].
type Profile struct {
	Version     int       `yaml:"version"`
	Name        string    `yaml:"name"`
	Tool        string    `yaml:"tool"`
	Description string    `yaml:"description"`
	Config      yaml.Node `yaml:"config"`
}

// Decode decodes the profile's Config section into the value pointed
// to by v. The caller supplies a tool-specific struct (e.g. config.Config).
//
// Example:
//
//	var cfg config.Config
//	err := profile.Decode(&cfg)
func (p *Profile) Decode(v any) error {
	if p.Config.Kind == 0 {
		// No config section present — leave v at its zero value.
		return nil
	}
	if err := p.Config.Decode(v); err != nil {
		return fmt.Errorf("decode profile %s config: %w", p.Name, err)
	}
	return nil
}

// ProfileInfo is a lightweight summary returned by Store.List.
type ProfileInfo struct {
	Name        string
	Tool        string
	Description string
	Builtin     bool
}
