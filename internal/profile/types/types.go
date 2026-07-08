package types

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
