package discovery

// DiscoveryTarget represents a target path to be discovered and the reason why.
type DiscoveryTarget struct {
	Path   string
	Reason string
}

// DiscoveryPlan represents an immutable execution plan detailing WHAT should be discovered.
// It contains no details about priority, queues, scheduling, or workers.
type DiscoveryPlan struct {
	Targets []DiscoveryTarget
	Phase   string // e.g. "bootstrap", "depth-0", "depth-1"
}

// NewDiscoveryPlan initializes a new DiscoveryPlan.
func NewDiscoveryPlan(targets []DiscoveryTarget, phase string) *DiscoveryPlan {
	return &DiscoveryPlan{
		Targets: targets,
		Phase:   phase,
	}
}
