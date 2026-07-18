package wordlist

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// Join appends path to base, preserving the base path and normalizing slashes.
//
// The base must be a clean directory root — no query string, no fragment.
// This is enforced because producers use base as a stable origin for recursion;
// arbitrary query/fragment components would silently corrupt child URLs.
func Join(base, path string) (string, error) {
	if strings.Contains(base, "?") || strings.Contains(base, "#") {
		return "", fmt.Errorf("base URL must not contain a query string or fragment")
	}

	cleanedPath := strings.TrimLeft(path, "/")

	isSimpleWord := true
	if cleanedPath == "" || cleanedPath == "." || cleanedPath == ".." {
		isSimpleWord = false
	} else {
		for i := 0; i < len(cleanedPath); i++ {
			c := cleanedPath[i]
			if c == '/' || c == '\\' || c == '?' || c == '#' || c == ':' {
				isSimpleWord = false
				break
			}
		}
	}

	if isSimpleWord {
		if strings.HasSuffix(base, "/") {
			return base + cleanedPath, nil
		}
		return base + "/" + cleanedPath, nil
	}

	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}
	if u.RawQuery != "" {
		return "", fmt.Errorf("base URL must not contain a query string: %q", base)
	}
	if u.Fragment != "" {
		return "", fmt.Errorf("base URL must not contain a fragment: %q", base)
	}

	rel, err := url.Parse(cleanedPath)
	if err != nil {
		return "", fmt.Errorf("invalid candidate path: %w", err)
	}

	// Strip fragment from candidate path
	rel.Fragment = ""

	// Ensure the base URL path ends with a slash so ResolveReference appends to it
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}

	resolved := u.ResolveReference(rel)
	return resolved.String(), nil
}

// CleanWord applies path policy checks and transformations to a word.
// It returns the cleaned word and false if it is rejected.
func CleanWord(word string, norm, collapse bool) (string, bool) {
	// Strip fragments
	if idx := strings.Index(word, "#"); idx != -1 {
		word = word[:idx]
	}

	if word == "" || word == "." || word == ".." {
		return "", false
	}
	if collapse {
		for strings.Contains(word, "//") {
			word = strings.ReplaceAll(word, "//", "/")
		}
	}
	if norm {
		word = path.Clean(word)
	}
	return word, true
}
