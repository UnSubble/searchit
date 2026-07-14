package discovery

// DiscoveryPlan represents an immutable execution plan produced by a Strategy.
// It outlines the specific paths to be enqueued and their priorities.
type DiscoveryPlan struct {
	Paths    []string
	Priority int
}

// NewDiscoveryPlan initializes a new DiscoveryPlan.
func NewDiscoveryPlan(paths []string, priority int) *DiscoveryPlan {
	return &DiscoveryPlan{
		Paths:    paths,
		Priority: priority,
	}
}
