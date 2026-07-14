package discovery_test

import (
	"sync"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/discovery"
)

func TestSignal_Creation(t *testing.T) {
	now := time.Now()
	metadata := map[string]string{"foo": "bar"}

	sig := discovery.Signal{
		Type:      discovery.SignalHeader,
		Source:    "X-Powered-By",
		Target:    "localhost:8080",
		Value:     "PHP/8.1",
		Timestamp: now,
		Metadata:  metadata,
	}

	if sig.Type != discovery.SignalHeader {
		t.Errorf("got Type %q, want %q", sig.Type, discovery.SignalHeader)
	}
	if sig.Source != "X-Powered-By" {
		t.Errorf("got Source %q, want %q", sig.Source, "X-Powered-By")
	}
	if sig.Target != "localhost:8080" {
		t.Errorf("got Target %q, want %q", sig.Target, "localhost:8080")
	}
	if sig.Value != "PHP/8.1" {
		t.Errorf("got Value %q, want %q", sig.Value, "PHP/8.1")
	}
	if !sig.Timestamp.Equal(now) {
		t.Errorf("got Timestamp %v, want %v", sig.Timestamp, now)
	}
	if sig.Metadata["foo"] != "bar" {
		t.Errorf("got Metadata[foo] %q, want %q", sig.Metadata["foo"], "bar")
	}
}

func TestRegistry_Lookup(t *testing.T) {
	r := discovery.NewRegistry()

	t.Run("lookup laravel", func(t *testing.T) {
		tech, ok := r.Lookup("laravel")
		if !ok {
			t.Fatal("expected laravel to be registered")
		}
		if tech.ID != "laravel" {
			t.Errorf("got ID %q, want %q", tech.ID, "laravel")
		}
		if tech.DisplayName != "Laravel" {
			t.Errorf("got DisplayName %q, want %q", tech.DisplayName, "Laravel")
		}
		if len(tech.InterestingFiles) != 2 {
			t.Errorf("got %d interesting files, want 2", len(tech.InterestingFiles))
		}
		if len(tech.InterestingCookies) != 1 || tech.InterestingCookies[0] != "laravel_session" {
			t.Errorf("got InterestingCookies %v, want [laravel_session]", tech.InterestingCookies)
		}
	})

	t.Run("lookup wordpress", func(t *testing.T) {
		tech, ok := r.Lookup("wordpress")
		if !ok {
			t.Fatal("expected wordpress to be registered")
		}
		if tech.ID != "wordpress" {
			t.Errorf("got ID %q, want %q", tech.ID, "wordpress")
		}
		if tech.DisplayName != "WordPress" {
			t.Errorf("got DisplayName %q, want %q", tech.DisplayName, "WordPress")
		}
		if len(tech.InterestingDirectories) != 3 {
			t.Errorf("got %d interesting directories, want 3", len(tech.InterestingDirectories))
		}
		if len(tech.InterestingHeaders) != 1 || tech.InterestingHeaders[0] != "X-Link" {
			t.Errorf("got InterestingHeaders %v, want [X-Link]", tech.InterestingHeaders)
		}
	})

	t.Run("lookup non-existent", func(t *testing.T) {
		_, ok := r.Lookup("unknown")
		if ok {
			t.Error("expected lookup of unknown technology to fail")
		}
	})
}

func TestBarrier_Creation(t *testing.T) {
	b := discovery.Barrier{
		Type: discovery.BarrierBootstrap,
	}

	if b.Type != discovery.BarrierBootstrap {
		t.Errorf("got Type %q, want %q", b.Type, discovery.BarrierBootstrap)
	}
}

func TestTargetContext_ConcurrencyAndIsolation(t *testing.T) {
	tc := discovery.NewTargetContext("a.com")
	if tc.Host != "a.com" {
		t.Errorf("got Host %q, want %q", tc.Host, "a.com")
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			tc.AddSignal(discovery.Signal{
				Type:   discovery.SignalCookie,
				Source: "cookie",
				Value:  "val",
			})
			tc.AddDiscovery("/path")
		}(i)
	}

	wg.Wait()

	signals := tc.GetSignals()
	if len(signals) != 100 {
		t.Errorf("got %d signals, want 100", len(signals))
	}
	if len(tc.Discoveries) != 100 {
		t.Errorf("got %d discoveries, want 100", len(tc.Discoveries))
	}
}

// MockStrategy implements discovery.Strategy interface for testing.
type MockStrategy struct{}

func (ms MockStrategy) Evaluate(tc *discovery.TargetContext, reg *discovery.Registry) *discovery.DiscoveryPlan {
	signals := tc.GetSignals()
	laravelMatched := false
	for _, sig := range signals {
		if sig.Type == discovery.SignalCookie && sig.Value == "laravel_session" {
			laravelMatched = true
			break
		}
	}

	if laravelMatched {
		if tech, ok := reg.Lookup("laravel"); ok {
			targets := make([]discovery.DiscoveryTarget, len(tech.InterestingFiles))
			for i, file := range tech.InterestingFiles {
				targets[i] = discovery.DiscoveryTarget{
					Path:   file,
					Reason: "Laravel interesting file",
				}
			}
			return discovery.NewDiscoveryPlan(targets, "depth-0")
		}
	}

	return discovery.NewDiscoveryPlan(nil, "bootstrap")
}

func TestStrategy_Evaluation(t *testing.T) {
	tc := discovery.NewTargetContext("example.com")
	reg := discovery.NewRegistry()
	strategy := MockStrategy{}

	// Initial evaluation should return empty/default plan
	p1 := strategy.Evaluate(tc, reg)
	if len(p1.Targets) != 0 {
		t.Errorf("got %d targets, want 0", len(p1.Targets))
	}
	if p1.Phase != "bootstrap" {
		t.Errorf("got phase %q, want %q", p1.Phase, "bootstrap")
	}

	// Add Laravel signal to trigger Laravel strategy decision
	tc.AddSignal(discovery.Signal{
		Type:  discovery.SignalCookie,
		Value: "laravel_session",
	})

	p2 := strategy.Evaluate(tc, reg)
	if len(p2.Targets) != 2 {
		t.Errorf("got %d targets, want 2", len(p2.Targets))
	}
	if p2.Phase != "depth-0" {
		t.Errorf("got phase %q, want %q", p2.Phase, "depth-0")
	}
	if p2.Targets[0].Path != ".env" || p2.Targets[0].Reason != "Laravel interesting file" {
		t.Errorf("got Targets[0] %+v, want path=.env reason=Laravel interesting file", p2.Targets[0])
	}

	// Bind plan to context and verify retrieval
	tc.AddDiscoveryPlan(p2)
	plans := tc.GetDiscoveryPlans()
	if len(plans) != 1 || plans[0].Phase != "depth-0" {
		t.Errorf("failed to retrieve bound discovery plans")
	}
}
