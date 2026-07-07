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
	"strings"

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

// Name represents a parsed and validated profile name.
type Name struct {
	Tool string
	Name string
}

// ParseName parses and validates a profile name.
func ParseName(s string) (Name, error) {
	if s == "" {
		return Name{}, fmt.Errorf("profile name cannot be empty")
	}
	if strings.Contains(s, "//") {
		return Name{}, fmt.Errorf("profile name cannot contain consecutive slashes")
	}
	if strings.HasPrefix(s, "/") {
		return Name{}, fmt.Errorf("profile name cannot start with a slash")
	}
	if strings.HasSuffix(s, "/") {
		return Name{}, fmt.Errorf("profile name cannot end with a slash")
	}
	parts := strings.Split(s, "/")
	if len(parts) < 2 {
		return Name{}, fmt.Errorf("profile name must include a namespace/tool (e.g. scan/myprofile)")
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return Name{}, fmt.Errorf("invalid segment %q in profile name", p)
		}
		if strings.ContainsAny(p, "\\:*?\"<>|") {
			return Name{}, fmt.Errorf("segment %q contains invalid characters", p)
		}
	}
	return Name{
		Tool: parts[0],
		Name: s,
	}, nil
}
