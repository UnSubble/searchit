package fingerprint

import "sync"

// Cache is a thread-safe store mapping normalized target hosts to their
// accumulated Fingerprint.
//
// Cache owns all Fingerprint instances it creates. Callers receive a pointer
// and may mutate the fingerprint (e.g. add signals) concurrently; they must
// not delete or replace it. Fingerprint lifetime is tied to the Cache that
// created it.
//
// The expected access pattern is write-once-per-host (on first encounter) and
// read-many (on every subsequent request to the same host). The implementation
// uses sync.RWMutex to reflect this: lookups acquire a read lock and only
// promote to a write lock when a new host is first seen.
//
// Cache is designed for a single scan lifetime. Create a new Cache per scan;
// do not share one across scans.
type Cache struct {
	mu    sync.RWMutex
	store map[string]*Fingerprint
}

// NewCache returns an empty, ready-to-use Cache.
func NewCache() *Cache {
	return &Cache{
		store: make(map[string]*Fingerprint),
	}
}

// Get returns the Fingerprint for host, or nil if no fingerprint has been
// recorded yet.
//
// host must be a normalized authority string (e.g. "https://example.com" or
// "example.com:8080"). Callers are responsible for normalizing before calling.
func (c *Cache) Get(host string) *Fingerprint {
	c.mu.RLock()
	fp := c.store[host]
	c.mu.RUnlock()
	return fp
}

// GetOrCreate returns the existing Fingerprint for host, creating and storing
// a new one if none exists.
//
// The returned pointer is stable: the same host always returns the same pointer
// for the lifetime of the Cache. This means callers can safely retain the
// pointer and call AddSignal on it without going through the cache again.
func (c *Cache) GetOrCreate(host string) *Fingerprint {
	// Fast path: already exists.
	c.mu.RLock()
	fp := c.store[host]
	c.mu.RUnlock()
	if fp != nil {
		return fp
	}

	// Slow path: first encounter for this host.
	c.mu.Lock()
	// Re-check after acquiring the write lock to handle concurrent first access.
	if fp = c.store[host]; fp == nil {
		fp = newFingerprint(host)
		c.store[host] = fp
	}
	c.mu.Unlock()
	return fp
}

// Len returns the number of distinct hosts currently tracked.
func (c *Cache) Len() int {
	c.mu.RLock()
	n := len(c.store)
	c.mu.RUnlock()
	return n
}

// Hosts returns a snapshot of all host keys currently in the cache.
// The returned slice is a copy; mutations do not affect the cache.
func (c *Cache) Hosts() []string {
	c.mu.RLock()
	out := make([]string, 0, len(c.store))
	for h := range c.store {
		out = append(out, h)
	}
	c.mu.RUnlock()
	return out
}
