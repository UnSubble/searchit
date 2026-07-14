package discovery_test

import (
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
		if tech.PriorityWeights[".env"] != 100 {
			t.Errorf("got .env weight %d, want 100", tech.PriorityWeights[".env"])
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
		if tech.PriorityWeights["wp-login.php"] != 100 {
			t.Errorf("got wp-login.php weight %d, want 100", tech.PriorityWeights["wp-login.php"])
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
