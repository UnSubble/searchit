package adaptive_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unsubble/searchit/internal/adaptive"
	"github.com/unsubble/searchit/internal/adaptive/types"
	"github.com/unsubble/searchit/internal/fingerprint"
)

func TestEngine_GetScore(t *testing.T) {
	// Setup mock server
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
	// Add mock Laravel header signal to fingerprint cache for the mock host
	u, _ := http.NewRequest("GET", srv.URL, nil)
	host := u.URL.Host

	fp := cache.GetOrCreate(host)
	fp.AddSignal(fingerprint.Signal{
		Source: "Header:X-Powered-By",
		Value:  "Laravel",
	})

	engine := adaptive.NewEngine(srv.URL, srv.Client(), cache, false)
	err := engine.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// 1. Laravel detected (+40) + robots.txt segment hit admin (+30) + robots.txt path match admin (+30) + admin word (+20) = 120
	score1 := engine.GetScore("admin", nil, 1, "text/html")
	if score1 != 120 {
		t.Errorf("expected score 120, got %d", score1)
	}

	// 2. Laravel detected (+40) + sitemap path api/users match (+30 + 30) = 100
	score2 := engine.GetScore("users", []string{"api"}, 2, "text/html")
	if score2 != 100 {
		t.Errorf("expected score 100, got %d", score2)
	}

	// 3. Laravel detected (+40) + robots.txt segment hit api (+30) + json content-type hint (+25) + json api word hit (+20) = 115
	score3 := engine.GetScore("api", nil, 1, "application/json")
	if score3 != 115 {
		t.Errorf("expected score 115, got %d", score3)
	}

	// 4. Laravel detected (+40) + framework specific word "storage" (+20) = 60
	score4 := engine.GetScore("storage", nil, 1, "text/html")
	if score4 != 60 {
		t.Errorf("expected score 60, got %d", score4)
	}

	// 5. SelectTraversal boundary verification (signal-driven)
	sigs := engine.GetSignals("api", nil, 1, "text/html")
	if engine.SelectTraversal(sigs).Policy != types.PolicyDFS {
		t.Errorf("expected DFS traversal for score %d, got %s", score4, engine.SelectTraversal(sigs).Policy)
	}
	sigs4 := engine.GetSignals("storage", nil, 1, "text/html")
	if engine.SelectTraversal(sigs4).Policy != types.PolicyDFS { // Has Laravel signal, which selects DFS!
		t.Errorf("expected DFS traversal for storage")
	}
}

func TestEngine_GetScore_WordPressAndExpress(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")
	fp.AddSignal(fingerprint.Signal{
		Source: "Header:X-Powered-By",
		Value:  "Express",
	})

	engine := adaptive.NewEngine("http://example.com", http.DefaultClient, cache, true)
	_ = engine.Discover(context.Background())

	// Express (+25) + Express word static (+20) + low priority folder static (+15) = 60
	score := engine.GetScore("static", nil, 1, "")
	if score != 60 {
		t.Errorf("expected Express score 60, got %d", score)
	}

	// Test WP
	cacheWP := fingerprint.NewCache()
	fpWP := cacheWP.GetOrCreate("example2.com")
	fpWP.AddSignal(fingerprint.Signal{
		Source: "Header:X-Powered-By",
		Value:  "WordPress",
	})

	engineWP := adaptive.NewEngine("http://example2.com", http.DefaultClient, cacheWP, true)
	_ = engineWP.Discover(context.Background())

	// WP (+35) + WP word wp-admin (+20) = 55
	scoreWP := engineWP.GetScore("wp-admin", nil, 1, "")
	if scoreWP != 55 {
		t.Errorf("expected WordPress score 55, got %d", scoreWP)
	}
}
