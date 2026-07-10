package resolver

import (
	"strings"
)

// resolveDepName resolves the name of a dependency based on its parent's name.
func resolveDepName(parentName, depName string) string {
	if strings.Contains(depName, "/") {
		return depName
	}
	parts := strings.SplitN(parentName, "/", 2)
	if len(parts) > 0 {
		return parts[0] + "/" + depName
	}
	return depName
}
