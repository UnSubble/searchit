package resolver

import (
	"fmt"
	"strings"
)

// CyclicDependencyError is returned when a cycle is detected in the dependency graph.
type CyclicDependencyError struct {
	Chain []string
}

func (e *CyclicDependencyError) Error() string {
	return fmt.Sprintf("cyclic profile dependency detected: %s", strings.Join(e.Chain, " -> "))
}
