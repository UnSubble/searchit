package targets

import "context"

// Target represents a single execution target in the system.
type Target struct {
	ID      int
	URL     string
	Method  string
	Body    string
	Headers []string
	Cookies string
}

// TargetContext binds a specific target to its local execution context.
type TargetContext struct {
	Ctx    context.Context
	Cancel context.CancelFunc
	Target *Target
}
