package discovery

// Strategy evaluates a target context and returns an execution DiscoveryPlan.
type Strategy interface {
	Evaluate(tc *TargetContext, reg *Registry) *DiscoveryPlan
}
