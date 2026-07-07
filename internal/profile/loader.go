package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Store provides read access to profiles from all configured sources.
type Store interface {
	// Load returns the profile with the given namespaced name (e.g. "scan/quick").
	// User profiles override embedded profiles on name collision.
	Load(name string) (*Profile, error)

	// List returns summary information for all discoverable profiles.
	// Results are sorted by name.
	List() ([]ProfileInfo, error)

	// LoadRaw returns the raw YAML bytes for the profile with the given name.
	LoadRaw(name string) ([]byte, error)
}

// DefaultStore resolves profiles from the user config directory and
// the embedded defaults. User profiles take precedence over embedded
// profiles with the same name.
type DefaultStore struct {
	// UserDir overrides the default user profile directory.
	// If empty, defaults to ~/.config/searchit/profiles.
	UserDir string
}

// NewStore returns a DefaultStore with standard paths.
func NewStore() *DefaultStore {
	return &DefaultStore{}
}

func (s *DefaultStore) userDir() string {
	if s.UserDir != "" {
		return s.UserDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "searchit", "profiles")
}

// Load returns the profile matching the given name. User profiles
// override embedded profiles on name collision.
func (s *DefaultStore) Load(name string) (*Profile, error) {
	// Check user profiles first.
	userProfiles, err := readUserProfiles(s.userDir())
	if err != nil {
		return nil, fmt.Errorf("load user profiles: %w", err)
	}
	if p, ok := userProfiles[name]; ok {
		return p, nil
	}

	// Fall back to embedded profiles.
	embedded, err := readEmbeddedProfiles()
	if err != nil {
		return nil, fmt.Errorf("load embedded profiles: %w", err)
	}
	if p, ok := embedded[name]; ok {
		return p, nil
	}

	return nil, fmt.Errorf("profile not found: %s", name)
}

// LoadRaw returns the raw YAML bytes for the profile with the given name.
// User profiles override embedded profiles.
func (s *DefaultStore) LoadRaw(name string) ([]byte, error) {
	p, err := s.Load(name)
	if err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal profile %s: %w", name, err)
	}
	return data, nil
}

// List returns summary information for all discoverable profiles,
// sorted by name. User profiles override embedded ones with the same name.
func (s *DefaultStore) List() ([]ProfileInfo, error) {
	embedded, err := readEmbeddedProfiles()
	if err != nil {
		return nil, fmt.Errorf("load embedded profiles: %w", err)
	}

	userProfiles, err := readUserProfiles(s.userDir())
	if err != nil {
		return nil, fmt.Errorf("load user profiles: %w", err)
	}

	// Build merged map. Embedded first, then user overrides.
	type entry struct {
		info    ProfileInfo
		builtin bool
	}
	merged := make(map[string]entry)

	for name, p := range embedded {
		merged[name] = entry{
			info: ProfileInfo{
				Name:        p.Name,
				Tool:        p.Tool,
				Description: p.Description,
				Builtin:     true,
			},
			builtin: true,
		}
	}

	for name, p := range userProfiles {
		merged[name] = entry{
			info: ProfileInfo{
				Name:        p.Name,
				Tool:        p.Tool,
				Description: p.Description,
				Builtin:     false,
			},
			builtin: false,
		}
	}

	result := make([]ProfileInfo, 0, len(merged))
	for _, e := range merged {
		result = append(result, e.info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}
