package adaptive

import (
	"github.com/unsubble/searchit/internal/adaptive/signals"
	"github.com/unsubble/searchit/internal/adaptive/types"
)

// Candidate represents a candidate path evaluation with accumulated signals.
type Candidate struct {
	Path        string
	Parent      string
	Signals     []signals.SignalType
	Policy      types.Policy
	PolicyRule  string // name of the selector rule that produced Policy
	ContentType string
}
