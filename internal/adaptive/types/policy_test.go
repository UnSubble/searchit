package types_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/adaptive/types"
)

func TestPolicy_String(t *testing.T) {
	tests := []struct {
		policy types.Policy
		want   string
	}{
		{types.PolicyBFS, "BFS"},
		{types.PolicyDFS, "DFS"},
		{types.PolicyEager, "EAGER"},
		{types.Policy(99), "BFS"}, // default fallback
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.policy.String(); got != tc.want {
				t.Errorf("Policy(%d).String() = %q, want %q", int(tc.policy), got, tc.want)
			}
		})
	}
}
