package semver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// versionRegex enforces strictly vX.Y.Z[-suffix]
	versionRegex = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([a-zA-Z0-9.-]+))?$`)
)

type Version struct {
	Original   string
	Major      int
	Minor      int
	Patch      int
	PreRelease string
}

// Parse extracts components from a strict vX.Y.Z semantic version string.
func Parse(v string) (Version, error) {
	v = strings.TrimSpace(v)
	matches := versionRegex.FindStringSubmatch(v)
	if matches == nil {
		return Version{}, fmt.Errorf("invalid semantic version format: %s (must be vX.Y.Z)", v)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	return Version{
		Original:   v,
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		PreRelease: matches[4],
	}, nil
}

// Compare returns 1 if v > other, -1 if v < other, and 0 if they are equal.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		if v.Major > other.Major {
			return 1
		}
		return -1
	}
	if v.Minor != other.Minor {
		if v.Minor > other.Minor {
			return 1
		}
		return -1
	}
	if v.Patch != other.Patch {
		if v.Patch > other.Patch {
			return 1
		}
		return -1
	}

	// If one is a prerelease and the other isn't, the prerelease is older.
	if v.PreRelease != "" && other.PreRelease == "" {
		return -1
	}
	if v.PreRelease == "" && other.PreRelease != "" {
		return 1
	}

	// Lexicographic comparison for prereleases (sufficient for our experimental tags)
	if v.PreRelease != other.PreRelease {
		if v.PreRelease > other.PreRelease {
			return 1
		}
		return -1
	}

	return 0
}

// IsStable returns true if there is no prerelease suffix.
func (v Version) IsStable() bool {
	return v.PreRelease == ""
}

// Channel returns "stable" or "experimental".
func (v Version) Channel() string {
	if v.IsStable() {
		return "stable"
	}
	return "experimental"
}

// Bump returns a new string version bumped according to the given type.
func (v Version) Bump(bumpType string) (string, error) {
	switch bumpType {
	case "patch":
		return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch+1), nil
	case "minor":
		return fmt.Sprintf("v%d.%d.0", v.Major, v.Minor+1), nil
	case "major":
		return fmt.Sprintf("v%d.0.0", v.Major+1), nil
	default:
		return "", fmt.Errorf("invalid bump type: %s", bumpType)
	}
}

// NextMajor returns the next major version string.
func (v Version) NextMajor() string {
	return fmt.Sprintf("v%d.0.0", v.Major+1)
}
