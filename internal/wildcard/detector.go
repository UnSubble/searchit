package wildcard

import (
	"fmt"
	"sync"
)

// Signature represents the fingerprint of a response body.
type Signature struct {
	StatusCode int
	BodyHash   uint64
	BodySize   int64
}

// Detector tracks response signatures to automatically detect wildcard behaviors.
type Detector struct {
	mu         sync.Mutex
	history    map[string][]Signature // key: "host:depth"
	wildcards  map[string]Signature   // key: "host:depth"
	minSamples int
	threshold  float64
}

// NewDetector creates a new wildcard Detector.
func NewDetector() *Detector {
	return &Detector{
		history:    make(map[string][]Signature),
		wildcards:  make(map[string]Signature),
		minSamples: 20,
		threshold:  0.8,
	}
}

func makeKey(host string, depth uint16) string {
	return fmt.Sprintf("%s:%d", host, depth)
}

// Add records a response signature at a given depth and returns the detected
// wildcard signature (if any) and a boolean indicating whether a wildcard is active.
func (d *Detector) Add(host string, depth uint16, sig Signature) (Signature, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := makeKey(host, depth)
	if wildcardSig, found := d.wildcards[key]; found {
		return wildcardSig, sig == wildcardSig
	}

	d.history[key] = append(d.history[key], sig)
	samples := d.history[key]

	if len(samples) >= d.minSamples {
		counts := make(map[Signature]int)
		for _, s := range samples {
			counts[s]++
		}

		var modalSig Signature
		maxCount := 0
		for s, count := range counts {
			if count > maxCount {
				maxCount = count
				modalSig = s
			}
		}

		ratio := float64(maxCount) / float64(len(samples))
		if ratio >= d.threshold {
			d.wildcards[key] = modalSig
			return modalSig, true
		}
	}

	return Signature{}, false
}

// IsWildcard returns true if the signature matches a detected wildcard at the given host and depth.
func (d *Detector) IsWildcard(host string, depth uint16, sig Signature) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := makeKey(host, depth)
	if wildcardSig, found := d.wildcards[key]; found {
		return sig == wildcardSig
	}
	return false
}
