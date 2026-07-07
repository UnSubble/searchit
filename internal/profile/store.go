package profile

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed embedded
var embeddedFS embed.FS

// readEmbeddedProfiles discovers and parses all .yaml files under the
// embedded/ directory. Profiles are keyed by their Name field.
func readEmbeddedProfiles() (map[string]*Profile, error) {
	profiles := make(map[string]*Profile)

	err := fs.WalkDir(embeddedFS, "embedded", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(d.Name())
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		data, err := embeddedFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded profile %s: %w", path, err)
		}

		var p Profile
		if err := yaml.Unmarshal(data, &p); err != nil {
			return fmt.Errorf("parse embedded profile %s: %w", path, err)
		}

		if p.Name == "" {
			return fmt.Errorf("embedded profile %s: missing name field", path)
		}

		// Validate that the name matches the file path convention.
		// e.g. embedded/scan/base.yaml → scan/base
		rel := strings.TrimPrefix(path, "embedded/")
		rel = strings.TrimSuffix(rel, ext)
		if p.Name != rel {
			return fmt.Errorf("embedded profile %s: name %q does not match path %q", path, p.Name, rel)
		}

		if _, exists := profiles[p.Name]; exists {
			return fmt.Errorf("duplicate embedded profile name: %s", p.Name)
		}

		profiles[p.Name] = &p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return profiles, nil
}

// readUserProfiles discovers and parses all .yaml files under the user
// profile directory. Returns an empty map if the directory does not exist.
func readUserProfiles(dir string) (map[string]*Profile, error) {
	profiles := make(map[string]*Profile)

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return profiles, nil
		}
		return nil, fmt.Errorf("stat user profile dir: %w", err)
	}
	if !info.IsDir() {
		return profiles, nil
	}

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(d.Name())
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read user profile %s: %w", path, err)
		}

		var p Profile
		if err := yaml.Unmarshal(data, &p); err != nil {
			return fmt.Errorf("parse user profile %s: %w", path, err)
		}

		if p.Name == "" {
			return fmt.Errorf("user profile %s: missing name field", path)
		}

		// Validate that the name matches the file path convention.
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("resolve relative path for %s: %w", path, err)
		}
		rel = filepath.ToSlash(rel)
		rel = strings.TrimSuffix(rel, ext)
		if p.Name != rel {
			return fmt.Errorf("user profile %s: name %q does not match path %q", path, p.Name, rel)
		}

		if _, exists := profiles[p.Name]; exists {
			return fmt.Errorf("duplicate user profile name: %s", p.Name)
		}

		profiles[p.Name] = &p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return profiles, nil
}
