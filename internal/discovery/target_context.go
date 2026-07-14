package discovery

import (
	"sync"
)

// TargetContext isolates the state, discoveries, and signals of a single target host.
type TargetContext struct {
	mu            sync.RWMutex
	Host          string
	Depth         int
	Signals       []Signal
	Discoveries   []string       // Paths discovered
	DiscoveryPlan *DiscoveryPlan // The current execution plan
}

// NewTargetContext initializes an isolated context for a host target.
func NewTargetContext(host string) *TargetContext {
	return &TargetContext{
		Host:        host,
		Signals:     make([]Signal, 0),
		Discoveries: make([]string, 0),
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

// SetDiscoveryPlan binds a new plan to this target context.
func (tc *TargetContext) SetDiscoveryPlan(plan *DiscoveryPlan) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.DiscoveryPlan = plan
}

// GetDiscoveryPlan retrieves the current plan.
func (tc *TargetContext) GetDiscoveryPlan() *DiscoveryPlan {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.DiscoveryPlan
}
