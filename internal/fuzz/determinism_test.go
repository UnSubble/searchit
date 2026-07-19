package fuzz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/stats"
)

func TestStrategies_Determinism(t *testing.T) {
	// Setup a mock HTTP server that simulates a typical target
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /admin\nDisallow: /api\n"))
			return
		}
		if path == "/sitemap.xml" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
				<url><loc>http://localhost/api/login</loc></url>
			</urlset>`))
			return
		}

		// Depth-based paths
		if path == "/admin" || path == "/api" || path == "/login" {
			if path == "/api" {
				w.Header().Set("Content-Type", "application/json")
			} else {
				w.Header().Set("Content-Type", "text/html")
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if path == "/admin/users" || path == "/api/login" || path == "/login/portal" {
			if path == "/api/login" {
				w.Header().Set("Content-Type", "application/json")
			} else {
				w.Header().Set("Content-Type", "text/html")
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if path == "/admin/users/profile" || path == "/api/login/debug" {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	fooWords := []string{"admin", "api", "login", "other1", "other2"}
	barWords := []string{"users", "login", "portal", "other3", "other4"}
	buzzWords := []string{"profile", "debug", "other5"}

	fs, err := filter.NewFilterSuite("200", "", "", "", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to create FilterSuite: %v", err)
	}

	scenarios := []struct {
		name     string
		strategy string
		adaptive bool
	}{
		{"eager", "eager", false},
		{"bfs", "bfs", false},
		{"dfs", "dfs", false},
		{"adaptive", "eager", true},
	}
	workerCounts := []int{1, 8, 32, 64, 128}

	for _, sc := range scenarios {
		t.Run(sc.name+" strategy worker counts", func(t *testing.T) {
			var baseline []string
			var firstRun = true

			for _, workers := range workerCounts {
				cache := fingerprint.NewCache()
				runner := &fuzz.Runner{
					TargetURL: srv.URL + "/FOO/BAR/BUZZ",
					Method:    "GET",
					FooWords:  fooWords,
					BarWords:  barWords,
					BuzzWords: buzzWords,
					Client:    srv.Client(),
					FS:        fs,
					Threads:   workers,
					Collector: stats.NewCollector(),
					Adaptive:  sc.adaptive,
					Cache:     cache,
				}

				var results []string
				err := runner.Run(context.Background(), sc.strategy, nil, func(res fuzz.Result) {
					results = append(results, strings.TrimPrefix(res.URL, srv.URL))
				})
				if err != nil {
					t.Fatalf("strategy %s failed for workers %d: %v", sc.name, workers, err)
				}

				if firstRun {
					baseline = results
					firstRun = false
					if len(baseline) == 0 {
						t.Fatalf("strategy %s generated 0 results, check mock server URLs", sc.name)
					}
				} else {
					if !reflect.DeepEqual(baseline, results) {
						t.Errorf("determinism violation for strategy %s between worker count 1 and %d\nBaseline: %v\nGot:      %v",
							sc.name, workers, baseline, results)
					}
				}
			}
		})

		t.Run(sc.name+" strategy consecutive runs", func(t *testing.T) {
			var firstResults []string
			runs := 50
			workers := 32

			for run := 0; run < runs; run++ {
				cache := fingerprint.NewCache()
				runner := &fuzz.Runner{
					TargetURL: srv.URL + "/FOO/BAR/BUZZ",
					Method:    "GET",
					FooWords:  fooWords,
					BarWords:  barWords,
					BuzzWords: buzzWords,
					Client:    srv.Client(),
					FS:        fs,
					Threads:   workers,
					Collector: stats.NewCollector(),
					Adaptive:  sc.adaptive,
					Cache:     cache,
				}

				var results []string
				err := runner.Run(context.Background(), sc.strategy, nil, func(res fuzz.Result) {
					results = append(results, strings.TrimPrefix(res.URL, srv.URL))
				})
				if err != nil {
					t.Fatalf("strategy %s run %d failed: %v", sc.name, run, err)
				}

				if run == 0 {
					firstResults = results
					if len(firstResults) == 0 {
						t.Fatalf("strategy %s first run generated 0 results", sc.name)
					}
				} else {
					if !reflect.DeepEqual(firstResults, results) {
						t.Fatalf("consecutive run %d for strategy %s produced different results\nRun 0: %v\nRun %d: %v",
							run, sc.name, firstResults, run, results)
					}
				}
			}
		})
	}
}
