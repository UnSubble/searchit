package v1

import "gopkg.in/yaml.v3"

// ProfileDoc represents the YAML structure of a schema v1 profile.
type ProfileDoc struct {
	Schema       int       `yaml:"schema"`
	Name         string    `yaml:"name"`
	Tool         string    `yaml:"tool"`
	Description  string    `yaml:"description"`
	Author       string    `yaml:"author"`
	Tags         []string  `yaml:"tags"`
	Homepage     string    `yaml:"homepage"`
	License      string    `yaml:"license"`
	Created      string    `yaml:"created"`
	Updated      string    `yaml:"updated"`
	Depends      []string  `yaml:"depends"`
	Experimental bool      `yaml:"experimental"`
	Config       yaml.Node `yaml:"config"`
}
