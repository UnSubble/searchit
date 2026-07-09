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

	// Create persists a new profile to the user config directory.
	Create(profile Profile) error
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
				Tags:        p.Tags,
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
				Tags:        p.Tags,
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

// Create persists a new profile to the user config directory.
// Returns an error if the profile name is invalid, if the profile already exists,
// or if writing to the filesystem fails.
func (s *DefaultStore) Create(profile Profile) error {
	name := profile.Name
	if _, err := ParseName(name); err != nil {
		return err
	}

	// 1. Check if it already exists as embedded
	embedded, err := readEmbeddedProfiles()
	if err != nil {
		return fmt.Errorf("read embedded profiles: %w", err)
	}
	if _, ok := embedded[name]; ok {
		return fmt.Errorf("profile %q already exists as a built-in profile", name)
	}

	userDir := s.userDir()
	if userDir == "" {
		return fmt.Errorf("could not resolve user profile directory")
	}

	// 2. Check if it already exists in the user profile directory
	filePathYAML := filepath.Join(userDir, name+".yaml")
	filePathYML := filepath.Join(userDir, name+".yml")
	if _, err := os.Stat(filePathYAML); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	}
	if _, err := os.Stat(filePathYML); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	}

	// 3. Create target directory
	dir := filepath.Dir(filePathYAML)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}

	// 4. Marshal and write profile
	data, err := yaml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}

	if err := os.WriteFile(filePathYAML, data, 0o644); err != nil {
		return fmt.Errorf("write profile file: %w", err)
	}

	return nil
}
