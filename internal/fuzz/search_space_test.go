package fuzz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/stats"
	"golang.org/x/time/rate"
)

func TestRunner_SearchSpaceProgress(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// "reject" triggers 404 which is filtered
		if strings.Contains(r.URL.Path, "reject") {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(200)
	}))
	defer ts.Close()

	fs, _ := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)

	tests := []struct {
		name          string
		url           string
		foo           []string
		bar           []string
		buzz          []string
		strategy      string
		expectedTotal int64
		expectPruning bool
	}{
		{
			name:          "Eager_SinglePlaceholder",
			url:           ts.URL + "/FUZZ",
			foo:           []string{"accept1", "reject1", "accept2"},
			strategy:      "eager",
			expectedTotal: 3,
			expectPruning: false, // eager executes all
		},
		{
			name:          "BFS_TwoPlaceholders_Pruning",
			url:           ts.URL + "/FOO/BAR",
			foo:           []string{"accept1", "reject1", "accept2"},
			bar:           []string{"bar1", "bar2", "bar3", "bar4"},
			strategy:      "bfs",
			expectedTotal: 12,
			expectPruning: true, // reject1 subtree (4 leaves) is pruned
		},
		{
			name:          "DFS_TwoPlaceholders_Pruning",
			url:           ts.URL + "/FOO/BAR",
			foo:           []string{"accept1", "reject1", "accept2"},
			bar:           []string{"bar1", "bar2", "bar3", "bar4"},
			strategy:      "dfs",
			expectedTotal: 12,
			expectPruning: true, // reject1 subtree (4 leaves) is pruned
		},
		{
			name:          "BFS_ThreePlaceholders_NestedPruning",
			url:           ts.URL + "/FOO/BAR/BUZZ",
			foo:           []string{"accept1", "reject1", "accept2"},
			bar:           []string{"accept1", "reject1"},
			buzz:          []string{"buzz1", "buzz2"},
			strategy:      "bfs",
			expectedTotal: 12,
			expectPruning: true,
		},
		{
			name:          "DFS_ThreePlaceholders_NestedPruning",
			url:           ts.URL + "/FOO/BAR/BUZZ",
			foo:           []string{"accept1", "reject1", "accept2"},
			bar:           []string{"accept1", "reject1"},
			buzz:          []string{"buzz1", "buzz2"},
			strategy:      "dfs",
			expectedTotal: 12,
			expectPruning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := stats.NewCollector()
			runner := &Runner{
				TargetURL: tt.url,
				FooWords:  tt.foo,
				BarWords:  tt.bar,
				BuzzWords: tt.buzz,
				Client:    ts.Client(),
				FS:        fs,
				Threads:   2,
				Delay:     0,
				Limiter:   rate.NewLimiter(rate.Inf, 1),
				Collector: collector,
			}

			// In our tests, FUZZ acts as primary if FOO is missing, but strategy uses FOO/BAR/BUZZ
			// The runner estimate candidates actually uses primary wordlist size for FUZZ.
			// Let's just override it.
			primarySize := len(tt.foo)
			if !strings.Contains(tt.url, "FUZZ") {
				primarySize = 0
			} else {
				runner.FooWords = nil
			}
			estimated := runner.EstimateCandidates(primarySize)
			if estimated != tt.expectedTotal {
				t.Fatalf("EstimateCandidates() = %d, want %d", estimated, tt.expectedTotal)
			}
			collector.SetTotalCandidates(estimated)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var yields int
			yield := func(res Result) {
				if res.Accepted {
					yields++
				}
			}

			var primaryChan <-chan string
			if strings.Contains(tt.url, "FUZZ") {
				pc := make(chan string, len(tt.foo))
				for _, w := range tt.foo {
					pc <- w
				}
				close(pc)
				primaryChan = pc
			}

			err := runner.Run(ctx, ctx, tt.strategy, primaryChan, yield)
			if err != nil {
				t.Fatalf("Runner.Run() error = %v", err)
			}

			snap := collector.Snapshot()
			if snap.TotalCandidates != tt.expectedTotal {
				t.Errorf("TotalCandidates = %d, want %d", snap.TotalCandidates, tt.expectedTotal)
			}
			if snap.SearchSpaceProgress != tt.expectedTotal {
				t.Errorf("SearchSpaceProgress = %d, want EXACTLY 100%% (%d) upon completion", snap.SearchSpaceProgress, tt.expectedTotal)
			}

			if tt.expectPruning {
				if snap.JobsProduced >= tt.expectedTotal {
					t.Errorf("Expected JobsProduced (%d) to be STRICTLY LESS THAN TotalCandidates (%d) due to pruning", snap.JobsProduced, tt.expectedTotal)
				}
			}
		})
	}
}
