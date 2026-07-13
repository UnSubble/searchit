package fingerprint_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/unsubble/searchit/internal/fingerprint"
)

// ---------------------------------------------------------------------------
// Fingerprint
// ---------------------------------------------------------------------------

func TestFingerprint_Host(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")
	if got := fp.Host(); got != "example.com" {
		t.Errorf("Host() = %q, want %q", got, "example.com")
	}
}

func TestFingerprint_AddSignal_And_Signals(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")

	if n := len(fp.Signals()); n != 0 {
		t.Fatalf("Signals() len = %d, want 0 on a fresh fingerprint", n)
	}

	s1 := fingerprint.Signal{Source: "header:Server", Value: "nginx", Confidence: fingerprint.Confidence(0.75)}
	s2 := fingerprint.Signal{Source: "cookie:name", Value: "PHPSESSID", Confidence: fingerprint.Confidence(0.50)}
	fp.AddSignal(s1)
	fp.AddSignal(s2)

	got := fp.Signals()
	if len(got) != 2 {
		t.Fatalf("Signals() len = %d, want 2", len(got))
	}
	if got[0] != s1 {
		t.Errorf("Signals()[0] = %+v, want %+v", got[0], s1)
	}
	if got[1] != s2 {
		t.Errorf("Signals()[1] = %+v, want %+v", got[1], s2)
	}
}

func TestFingerprint_Signals_ReturnsCopy(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("example.com")
	fp.AddSignal(fingerprint.Signal{Source: "header:X-Powered-By", Value: "PHP/8.1", Confidence: fingerprint.Confidence(0.75)})

	snap := fp.Signals()
	// Mutate the returned slice; the fingerprint must be unaffected.
	snap[0] = fingerprint.Signal{Source: "poisoned", Value: "poisoned", Confidence: 0}

	after := fp.Signals()
	if after[0].Source == "poisoned" {
		t.Error("Signals() returned a live slice; mutations must not affect the fingerprint")
	}
}

func TestFingerprint_ConcurrentAddSignal(t *testing.T) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("concurrent.example.com")

	const goroutines = 50
	const signalsEach = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < signalsEach; j++ {
				fp.AddSignal(fingerprint.Signal{
					Source:     fmt.Sprintf("worker:%d", id),
					Value:      fmt.Sprintf("val-%d", j),
					Confidence: fingerprint.Confidence(0.25),
				})
			}
		}(i)
	}
	wg.Wait()

	if got := len(fp.Signals()); got != goroutines*signalsEach {
		t.Errorf("Signals() len = %d, want %d after concurrent writes", got, goroutines*signalsEach)
	}
}

// ---------------------------------------------------------------------------
// Cache
// ---------------------------------------------------------------------------

func TestCache_GetOrCreate_SamePointer(t *testing.T) {
	cache := fingerprint.NewCache()
	fp1 := cache.GetOrCreate("example.com")
	fp2 := cache.GetOrCreate("example.com")
	if fp1 != fp2 {
		t.Error("GetOrCreate returned different pointers for the same host")
	}
}

func TestCache_Get_MissingHost(t *testing.T) {
	cache := fingerprint.NewCache()
	if got := cache.Get("missing.example.com"); got != nil {
		t.Errorf("Get on absent host = %v, want nil", got)
	}
}

func TestCache_Get_AfterCreate(t *testing.T) {
	cache := fingerprint.NewCache()
	created := cache.GetOrCreate("example.com")
	got := cache.Get("example.com")
	if got != created {
		t.Error("Get after GetOrCreate returned a different pointer")
	}
}

func TestCache_Len(t *testing.T) {
	cache := fingerprint.NewCache()
	if n := cache.Len(); n != 0 {
		t.Fatalf("Len() = %d on empty cache, want 0", n)
	}
	cache.GetOrCreate("a.example.com")
	cache.GetOrCreate("b.example.com")
	cache.GetOrCreate("a.example.com") // duplicate; must not increase count
	if n := cache.Len(); n != 2 {
		t.Errorf("Len() = %d, want 2", n)
	}
}

func TestCache_Hosts(t *testing.T) {
	cache := fingerprint.NewCache()
	cache.GetOrCreate("alpha.example.com")
	cache.GetOrCreate("beta.example.com")

	hosts := cache.Hosts()
	if len(hosts) != 2 {
		t.Fatalf("Hosts() len = %d, want 2", len(hosts))
	}
	set := make(map[string]bool)
	for _, h := range hosts {
		set[h] = true
	}
	if !set["alpha.example.com"] || !set["beta.example.com"] {
		t.Errorf("Hosts() = %v, missing expected entries", hosts)
	}
}

func TestCache_Hosts_ReturnsCopy(t *testing.T) {
	cache := fingerprint.NewCache()
	cache.GetOrCreate("example.com")
	snap := cache.Hosts()
	snap[0] = "poisoned"
	if cache.Hosts()[0] == "poisoned" {
		t.Error("Hosts() returned a live slice; mutations must not affect the cache")
	}
}

func TestCache_ConcurrentGetOrCreate(t *testing.T) {
	cache := fingerprint.NewCache()
	const goroutines = 100
	const hosts = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)
	ptrs := make([]*fingerprint.Fingerprint, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			host := fmt.Sprintf("host%d.example.com", id%hosts)
			ptrs[id] = cache.GetOrCreate(host)
		}(i)
	}
	wg.Wait()

	// Verify pointer stability: same host must always return the same pointer.
	hostPtrs := make(map[string]*fingerprint.Fingerprint)
	for i := 0; i < goroutines; i++ {
		host := fmt.Sprintf("host%d.example.com", i%hosts)
		if existing, ok := hostPtrs[host]; ok {
			if existing != ptrs[i] {
				t.Errorf("host %s returned different pointers across concurrent calls", host)
			}
		} else {
			hostPtrs[host] = ptrs[i]
		}
	}

	if n := cache.Len(); n != hosts {
		t.Errorf("Len() = %d after concurrent access, want %d", n, hosts)
	}
}

func TestFingerprint_Robustness(t *testing.T) {
	cache := fingerprint.NewCache()

	// 1. Empty string lookup
	fpEmpty := cache.GetOrCreate("")
	if fpEmpty == nil {
		t.Error("expected GetOrCreate(\"\") to return a fingerprint instance, got nil")
	}

	// 2. Write identical signals repeatedly to ensure stability
	const repetitions = 200
	sig := fingerprint.Signal{Source: "test", Value: "duplicate", Confidence: 1.0}
	for i := 0; i < repetitions; i++ {
		fpEmpty.AddSignal(sig)
	}
	signals := fpEmpty.Signals()
	if len(signals) != repetitions {
		t.Errorf("expected %d signals, got %d", repetitions, len(signals))
	}

	// 3. High concurrency read and write stress
	const workers = 30
	const actions = 100
	var wg sync.WaitGroup
	wg.Add(workers * 2)

	// Write workers
	for i := 0; i < workers; i++ {
		go func(wID int) {
			defer wg.Done()
			for a := 0; a < actions; a++ {
				fp := cache.GetOrCreate(fmt.Sprintf("host-%d.com", wID%3))
				fp.AddSignal(fingerprint.Signal{
					Source:     fmt.Sprintf("source-%d", a),
					Value:      "value",
					Confidence: 0.5,
				})
			}
		}(i)
	}

	// Read workers
	for i := 0; i < workers; i++ {
		go func(wID int) {
			defer wg.Done()
			for a := 0; a < actions; a++ {
				fp := cache.Get(fmt.Sprintf("host-%d.com", wID%3))
				if fp != nil {
					_ = fp.Signals()
				}
			}
		}(i)
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkCache_GetOrCreate_Existing(b *testing.B) {
	cache := fingerprint.NewCache()
	cache.GetOrCreate("bench.example.com")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.GetOrCreate("bench.example.com")
	}
}

func BenchmarkFingerprint_AddSignal(b *testing.B) {
	cache := fingerprint.NewCache()
	fp := cache.GetOrCreate("bench.example.com")
	s := fingerprint.Signal{Source: "header:Server", Value: "nginx", Confidence: fingerprint.Confidence(0.75)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fp.AddSignal(s)
	}
}
