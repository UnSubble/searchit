package discovery

// Decision details an execution adjustment decided by a Strategy.
type Decision struct {
	Priority int
	Paths    []string
}

// Strategy evaluates a target context and returns decisions for future execution.
type Strategy interface {
	Evaluate(tc *TargetContext, reg *Registry) Decision
}
