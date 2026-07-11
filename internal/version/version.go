package version

import (
	"fmt"
	"strings"
)

const Name = "searchit"

// Defined as variables so they can be overridden via -ldflags at build time.
var (
	Version = "v0.3.0"
	Commit  = "dev"
	Date    = "unknown"
)

// String returns a concise version string.
func String() string {
	v := Version
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return Name + " " + v
}

// Long returns a detailed build metadata version string.
func Long() string {
	v := Version
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return fmt.Sprintf("%s %s\nCommit: %s\nBuilt: %s", Name, v, Commit, Date)
}
