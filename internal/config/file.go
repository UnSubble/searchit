package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// File represents the global configuration file schema.
// It uses strict decoding to reject unknown keys.
type File struct {
	Scan *ScanOverlay `yaml:"scan"`
	Fuzz *FuzzOverlay `yaml:"fuzz"`
}

// ResolveConfigPath returns the configuration file path according to XDG spec.
// If customPath is non-empty, it is returned directly.
// Otherwise, it checks $XDG_CONFIG_HOME/searchit/config.yaml, falling back to ~/.config/searchit/config.yaml.
func ResolveConfigPath(customPath string) (string, bool) {
	if customPath != "" {
		return customPath, true
	}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		p := filepath.Join(xdg, "searchit", "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p, false
		}
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		p := filepath.Join(home, ".config", "searchit", "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p, false
		}
	}

	return "", false
}

// LoadFile reads and parses a global configuration YAML file.
// If explicitPath is provided and the file does not exist or fails validation, an error is returned.
// If explicitPath is empty and the default XDG/home file does not exist, (nil, "", nil) is returned.
func LoadFile(explicitPath string) (*File, string, error) {
	resolvedPath, isExplicit := ResolveConfigPath(explicitPath)
	if resolvedPath == "" {
		return nil, "", nil
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		if isExplicit {
			return nil, resolvedPath, fmt.Errorf("config file error: %w", err)
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil, resolvedPath, nil
		}
		return nil, resolvedPath, fmt.Errorf("failed to read default config file %q: %w", resolvedPath, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var f File
	if err := decoder.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return &File{}, resolvedPath, nil
		}
		return nil, resolvedPath, fmt.Errorf("config file validation error in %q: %w", resolvedPath, err)
	}

	return &f, resolvedPath, nil
}
