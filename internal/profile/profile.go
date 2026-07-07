// Package profile implements Searchit's profile management system.
//
// Profiles are first-class, tool-agnostic resources that wrap config.Config
// with metadata (name, tool, description). They are resolved from user
// directories and embedded defaults, with user profiles taking precedence.
package profile

// ProfileConfig holds the subset of configuration fields that a profile
// can specify. Fields use YAML tags for serialisation.
//
// This struct intentionally mirrors the shape of config.Config but only
// includes fields that are meaningful in a profile context. Runtime-only
// fields (URLs, output format, etc.) are excluded.
type ProfileConfig struct {
	Threads       int    `yaml:"threads,omitempty"`
	Timeout       int    `yaml:"timeout,omitempty"`
	Recursive     bool   `yaml:"recursive,omitempty"`
	MaxDepth      uint16 `yaml:"max_depth,omitempty"`
	ExcludeStatus string `yaml:"exclude_status,omitempty"`
}

// Profile is the top-level profile document.
type Profile struct {
	Version     int           `yaml:"version"`
	Name        string        `yaml:"name"`
	Tool        string        `yaml:"tool"`
	Description string        `yaml:"description"`
	Config      ProfileConfig `yaml:"config"`
}

// ProfileInfo is a lightweight summary returned by Store.List.
type ProfileInfo struct {
	Name        string
	Tool        string
	Description string
	Builtin     bool
}
