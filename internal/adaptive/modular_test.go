package adaptive_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/adaptive"
	"github.com/unsubble/searchit/internal/adaptive/prioritizer"
	"github.com/unsubble/searchit/internal/adaptive/selector"
	"github.com/unsubble/searchit/internal/adaptive/signals"
	"github.com/unsubble/searchit/internal/adaptive/summary"
	"github.com/unsubble/searchit/internal/adaptive/types"
	"github.com/unsubble/searchit/internal/fingerprint"
)

func TestModular_SignalsAndPrioritizer(t *testing.T) {
	// 1. Direct validation of ScoreMap constants
	if signals.ScoreMap[signals.SignalLaravel] != 40 {
		t.Errorf("expected Laravel score 40, got %d", signals.ScoreMap[signals.SignalLaravel])
	}
	if signals.ScoreMap[signals.SignalRobots] != 30 {
		t.Errorf("expected RobotsTxt score 30, got %d", signals.ScoreMap[signals.SignalRobots])
	}

	// 2. Direct validation of GetScore
	sigs := []signals.SignalType{signals.SignalLaravel, signals.SignalRobots}
	if prioritizer.GetScore(sigs) != 70 {
		t.Errorf("expected GetScore sum 70, got %d", prioritizer.GetScore(sigs))
	}

	// 3. Direct validation of CalculateSignals
	pSegments := map[string]bool{"admin": true}
	pPaths := map[string]bool{"admin/login": true}

	calc := prioritizer.CalculateSignals(
		"login",
		[]string{"admin"},
		2,
		"application/json",
		pSegments,
		pPaths,
		true,  // laravel
		false, // wp
		false, // express
	)

	hasLaravel := false
	hasJson := false
	hasSitemap := false
	for _, s := range calc {
		if s == signals.SignalLaravel {
			hasLaravel = true
		}
		if s == signals.SignalJSON {
			hasJson = true
		}
		if s == signals.SignalSitemap {
			hasSitemap = true
		}
	}
	if !hasLaravel || !hasJson || !hasSitemap {
		t.Errorf("CalculateSignals missing expected signals: %v", calc)
	}
}

func TestModular_Selector(t *testing.T) {
	// Laravel signal → DFS via "deep-tree" rule
	d := selector.Select(selector.DefaultRules, []signals.SignalType{signals.SignalLaravel})
	if d.Policy != types.PolicyDFS {
		t.Errorf("expected DFS for Laravel, got %s", d.Policy)
	}
	if d.Rule == "" {
		t.Error("expected non-empty Rule name for DFS decision")
	}

	// No signals → BFS default
	d2 := selector.Select(selector.DefaultRules, []signals.SignalType{})
	if d2.Policy != types.PolicyBFS {
		t.Errorf("expected BFS for empty signals, got %s", d2.Policy)
	}
	if d2.Rule != "default" {
		t.Errorf("expected rule \"default\", got %q", d2.Rule)
	}

	// Asset signal → EAGER
	d3 := selector.Select(selector.DefaultRules, []signals.SignalType{signals.SignalAsset})
	if d3.Policy != types.PolicyEager {
		t.Errorf("expected EAGER for asset, got %s", d3.Policy)
	}
}

func TestModular_Summary(t *testing.T) {
	s := summary.NewSummary()
	s.Technologies = []string{"Laravel", "Express"}
	s.Discoveries = []string{"robots.txt", "sitemap.xml"}
	s.RecordTraversal(types.PolicyDFS)
	s.RecordTraversal(types.PolicyBFS)
	s.RecordTraversal(types.PolicyEager)
	s.RecordFindings(27)

	var buf bytes.Buffer
	s.Print(&buf)
	output := buf.String()

	requiredSubstrings := []string{
		"ADAPTIVE SUMMARY",
		"Laravel",
		"Express",
		"robots.txt",
		"sitemap.xml",
		"DFS .......... 1",
		"BFS ......... 1",
		"EAGER ........ 1",
		"Candidates:",
		"    3",
		"Findings:",
		"    27",
	}

	for _, sub := range requiredSubstrings {
		if !strings.Contains(output, sub) {
			t.Errorf("Summary output missing expected string %q in:\n%s", sub, output)
		}
	}
}

func TestModular_CollectorAndEngine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /admin\n"))
			return
		}
		if p == "/sitemap.xml" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
				<url><loc>/api/users</loc></url>
			</urlset>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cache := fingerprint.NewCache()
	u, _ := http.NewRequest("GET", srv.URL, nil)
	host := u.URL.Host
	fp := cache.GetOrCreate(host)
	fp.AddSignal(fingerprint.Signal{
		Source: "Header:X-Powered-By",
		Value:  "Express",
	})

	engine := adaptive.NewEngine(srv.URL, srv.Client(), cache, true)
	err := engine.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if !engine.Collector.ExpressDetected {
		t.Error("expected Express to be detected")
	}
	if !engine.Collector.RobotsDiscovered {
		t.Error("expected robots.txt to be discovered")
	}
	if !engine.Collector.SitemapDiscovered {
		t.Error("expected sitemap.xml to be discovered")
	}

	// Express (+25) + sitemap segment api match (+30) + Express word api (+20) = 75 -> DFS (since it has Express/Sitemap/API signals)
	score := engine.GetScore("api", nil, 1, "text/html")
	if score != 75 {
		t.Errorf("expected score 75, got %d", score)
	}
	sigs := engine.GetSignals("api", nil, 1, "text/html")
	if engine.SelectTraversal(sigs).Policy != types.PolicyDFS {
		t.Errorf("expected DFS traversal for score %d, got %s", score, engine.SelectTraversal(sigs).Policy)
	}
}
