package wordlist

import (
	"fmt"
	"net/url"
	"strings"
)

// Join appends path to base, preserving the base path and normalizing slashes.
//
// The base must be a clean directory root — no query string, no fragment.
// This is enforced because producers use base as a stable origin for recursion;
// arbitrary query/fragment components would silently corrupt child URLs.
func Join(base, path string) (string, error) {
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

	// Strip trailing slashes from the base path and leading slashes from the
	// appended path so there is always exactly one slash at the join point,
	// regardless of how the caller supplies either side.
	basePath := strings.TrimRight(u.Path, "/")
	u.Path = basePath + "/" + strings.TrimLeft(path, "/")

	return u.String(), nil
}
