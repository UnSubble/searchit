package discovery

import (
	"sync"
)

// TargetContext isolates the state, discoveries, plans, and signals of a single target host.
type TargetContext struct {
	mu             sync.RWMutex
	Host           string
	Depth          int
	Signals        []Signal
	Discoveries    []string
	DiscoveryPlans []*DiscoveryPlan // Chronological log of intent plans
}

// NewTargetContext initializes an isolated context for a host target.
func NewTargetContext(host string) *TargetContext {
	return &TargetContext{
		Host:           host,
		Signals:        make([]Signal, 0),
		Discoveries:    make([]string, 0),
		DiscoveryPlans: make([]*DiscoveryPlan, 0),
	}
}

// AddSignal appends a signal to the context thread-safely.
func (tc *TargetContext) AddSignal(s Signal) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.Signals = append(tc.Signals, s)
}

// GetSignals returns a snapshot copy of all signals.
func (tc *TargetContext) GetSignals() []Signal {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	out := make([]Signal, len(tc.Signals))
	copy(out, tc.Signals)
	return out
}

// AddDiscovery records a discovered path.
func (tc *TargetContext) AddDiscovery(path string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.Discoveries = append(tc.Discoveries, path)
}

// AddDiscoveryPlan appends a new plan to this target context.
func (tc *TargetContext) AddDiscoveryPlan(plan *DiscoveryPlan) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.DiscoveryPlans = append(tc.DiscoveryPlans, plan)
}

// GetDiscoveryPlans retrieves the chronological slice of plans.
func (tc *TargetContext) GetDiscoveryPlans() []*DiscoveryPlan {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	out := make([]*DiscoveryPlan, len(tc.DiscoveryPlans))
	copy(out, tc.DiscoveryPlans)
	return out
}
