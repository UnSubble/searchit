package selector_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/adaptive/selector"
	"github.com/unsubble/searchit/internal/adaptive/signals"
	"github.com/unsubble/searchit/internal/adaptive/types"
)

func TestSelect_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		rules      []selector.Rule
		sigs       []signals.SignalType
		wantPolicy types.Policy
		wantRule   string
	}{
		{
			name:       "Default rules with no signals returns BFS default",
			rules:      selector.DefaultRules,
			sigs:       nil,
			wantPolicy: types.PolicyBFS,
			wantRule:   "default",
		},
		{
			name:       "Deep tree signal matches DFS",
			rules:      selector.DefaultRules,
			sigs:       []signals.SignalType{signals.SignalLaravel},
			wantPolicy: types.PolicyDFS,
			wantRule:   "deep-tree (framework/API/admin/discovery)",
		},
		{
			name:       "Asset signal matches Eager",
			rules:      selector.DefaultRules,
			sigs:       []signals.SignalType{signals.SignalAsset},
			wantPolicy: types.PolicyEager,
			wantRule:   "static-asset",
		},
		{
			name:       "Precedence: deep-tree beats asset",
			rules:      selector.DefaultRules,
			sigs:       []signals.SignalType{signals.SignalAsset, signals.SignalAPI},
			wantPolicy: types.PolicyDFS,
			wantRule:   "deep-tree (framework/API/admin/discovery)",
		},
		{
			name: "Custom rules matching first rule",
			rules: []selector.Rule{
				{
					Name:       "custom-admin",
					RequireAny: []signals.SignalType{signals.SignalAdmin},
					Policy:     types.PolicyDFS,
				},
			},
			sigs:       []signals.SignalType{signals.SignalAdmin},
			wantPolicy: types.PolicyDFS,
			wantRule:   "custom-admin",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dec := selector.Select(tc.rules, tc.sigs)
			if dec.Policy != tc.wantPolicy {
				t.Errorf("got policy %v, want %v", dec.Policy, tc.wantPolicy)
			}
			if dec.Rule != tc.wantRule {
				t.Errorf("got rule %q, want %q", dec.Rule, tc.wantRule)
			}
		})
	}
}
