package types

// Policy is a strongly typed traversal policy selected by the Adaptive Engine.
// It replaces the string constants "DFS", "BFS", and "EAGER" previously used
// throughout the adaptive layer.
type Policy uint8

const (
	// PolicyBFS is the default traversal policy when no strong signal is present.
	PolicyBFS Policy = iota
	// PolicyDFS is selected when deep-tree signals are detected (frameworks, APIs, admin paths).
	PolicyDFS
	// PolicyEager is selected when low-priority asset signals dominate.
	PolicyEager
)

// String returns the canonical human-readable name of the policy.
func (p Policy) String() string {
	switch p {
	case PolicyDFS:
		return "DFS"
	case PolicyEager:
		return "EAGER"
	default:
		return "BFS"
	}
}
