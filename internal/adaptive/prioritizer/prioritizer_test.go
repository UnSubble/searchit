package prioritizer_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/adaptive/prioritizer"
	"github.com/unsubble/searchit/internal/adaptive/signals"
)

func TestCalculateSignals_TableDriven(t *testing.T) {
	tests := []struct {
		name                 string
		word                 string
		parentPath           []string
		depth                int
		parentResContentType string
		prioritizedSegments  map[string]bool
		prioritizedPaths     map[string]bool
		laravel              bool
		wp                   bool
		express              bool
		wantSignals          []signals.SignalType
	}{
		{
			name:                "Robots segment match",
			word:                "secret-admin",
			prioritizedSegments: map[string]bool{"secret-admin": true},
			wantSignals:         []signals.SignalType{signals.SignalRobots},
		},
		{
			name:             "Sitemap full path match",
			word:             "sitemap-page",
			parentPath:       []string{"docs"},
			prioritizedPaths: map[string]bool{"docs/sitemap-page": true},
			wantSignals:      []signals.SignalType{signals.SignalSitemap},
		},
		{
			name:        "Laravel technology with horizon path",
			word:        "horizon",
			laravel:     true,
			wantSignals: []signals.SignalType{signals.SignalLaravel, signals.SignalAdmin},
		},
		{
			name:        "WordPress technology with wp-admin path",
			word:        "wp-admin",
			wp:          true,
			wantSignals: []signals.SignalType{signals.SignalWordPress, signals.SignalAdmin},
		},
		{
			name:        "Express technology with api path",
			word:        "api",
			express:     true,
			wantSignals: []signals.SignalType{signals.SignalExpress, signals.SignalAdmin},
		},
		{
			name:                 "JSON Content-Type with graphql",
			word:                 "graphql",
			parentResContentType: "application/json; charset=utf-8",
			wantSignals:          []signals.SignalType{signals.SignalJSON, signals.SignalGraphQL},
		},
		{
			name:                 "JSON Content-Type with v1",
			word:                 "v1",
			parentResContentType: "application/json",
			wantSignals:          []signals.SignalType{signals.SignalJSON, signals.SignalAPI},
		},
		{
			name:        "Low priority static assets folder",
			word:        "images",
			wantSignals: []signals.SignalType{signals.SignalAsset},
		},
		{
			name:        "Explicit admin word",
			word:        "admin",
			wantSignals: []signals.SignalType{signals.SignalAdmin},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := prioritizer.CalculateSignals(
				tc.word,
				tc.parentPath,
				tc.depth,
				tc.parentResContentType,
				tc.prioritizedSegments,
				tc.prioritizedPaths,
				tc.laravel,
				tc.wp,
				tc.express,
			)

			if len(got) != len(tc.wantSignals) {
				t.Fatalf("expected %d signals %v, got %d %v", len(tc.wantSignals), tc.wantSignals, len(got), got)
			}

			for i, sig := range tc.wantSignals {
				if got[i] != sig {
					t.Errorf("signal %d mismatch: got %v, want %v", i, got[i], sig)
				}
			}
		})
	}
}

func TestGetScore(t *testing.T) {
	sigs := []signals.SignalType{
		signals.SignalLaravel, // 40
		signals.SignalRobots,  // 30
		signals.SignalAdmin,   // 20
	}

	score := prioritizer.GetScore(sigs)
	if score != 90 {
		t.Errorf("expected score 90, got %d", score)
	}

	if emptyScore := prioritizer.GetScore(nil); emptyScore != 0 {
		t.Errorf("expected 0 for empty signals, got %d", emptyScore)
	}
}
