package signals_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/adaptive/signals"
)

func TestSignalScoreMap(t *testing.T) {
	expectedSignals := []signals.SignalType{
		signals.SignalRobots,
		signals.SignalSitemap,
		signals.SignalLaravel,
		signals.SignalWordPress,
		signals.SignalExpress,
		signals.SignalJSON,
		signals.SignalAPI,
		signals.SignalGraphQL,
		signals.SignalAdmin,
		signals.SignalAsset,
	}

	for _, sig := range expectedSignals {
		score, ok := signals.ScoreMap[sig]
		if !ok {
			t.Errorf("missing score for signal %q", sig)
		}
		if score <= 0 {
			t.Errorf("score for signal %q must be positive, got %d", sig, score)
		}
	}
}
