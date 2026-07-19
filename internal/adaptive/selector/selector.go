package selector

import (
	"github.com/unsubble/searchit/internal/adaptive/signals"
	"github.com/unsubble/searchit/internal/adaptive/types"
)

// Rule is a declarative traversal rule.
// Rules are evaluated top-to-bottom; the first matching rule wins.
type Rule struct {
	// Name is a human-readable label included in every Decision for explainability.
	Name string
	// RequireAny fires the rule when ANY of the listed signals are present.
	RequireAny []signals.SignalType
	// Policy is the traversal policy applied when the rule matches.
	Policy types.Policy
}

// Decision is the result of a rule evaluation.
type Decision struct {
	Policy types.Policy
	// Rule is the name of the matched rule. "default" when no rule matched.
	Rule string
}

// DefaultRules is the canonical, ordered rule table used by the Adaptive Engine.
// Rules are evaluated top-to-bottom; the first match wins.
//
// Order rationale:
//  1. Deep-tree signals (frameworks, APIs, admin paths) → DFS  (most valuable branches)
//  2. Static-asset signals                              → EAGER (flat, no deep benefit)
//  3. Implicit default                                  → BFS   (broad unknown targets)
var DefaultRules = []Rule{
	{
		Name: "deep-tree (framework/API/admin/discovery)",
		RequireAny: []signals.SignalType{
			signals.SignalLaravel,
			signals.SignalWordPress,
			signals.SignalExpress,
			signals.SignalJSON,
			signals.SignalAPI,
			signals.SignalGraphQL,
			signals.SignalAdmin,
			signals.SignalRobots,
			signals.SignalSitemap,
		},
		Policy: types.PolicyDFS,
	},
	{
		Name:       "static-asset",
		RequireAny: []signals.SignalType{signals.SignalAsset},
		Policy:     types.PolicyEager,
	},
}

// Select evaluates rules against the provided signals and returns a Decision.
// If no rule matches, the decision defaults to PolicyBFS.
func Select(rules []Rule, sigs []signals.SignalType) Decision {
	sigSet := make(map[signals.SignalType]struct{}, len(sigs))
	for _, s := range sigs {
		sigSet[s] = struct{}{}
	}

	for _, rule := range rules {
		for _, required := range rule.RequireAny {
			if _, ok := sigSet[required]; ok {
				return Decision{Policy: rule.Policy, Rule: rule.Name}
			}
		}
	}

	return Decision{Policy: types.PolicyBFS, Rule: "default"}
}
