package v1

import "gopkg.in/yaml.v3"

// ProfileDoc represents the YAML structure of a schema v1 profile.
type ProfileDoc struct {
	Schema      int       `yaml:"schema"`
	Name        string    `yaml:"name"`
	Tool        string    `yaml:"tool"`
	Description string    `yaml:"description"`
	Config      yaml.Node `yaml:"config"`
}
