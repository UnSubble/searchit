package fingerprint

import "sync"

// Confidence is a detection confidence score in the range [0, 1].
// 0 means no evidence; 1 means certain.
//
// float32 is used intentionally: sub-percent precision is not meaningful for
// detection confidence, and the smaller width reduces struct size when many
// signals accumulate per fingerprint.
type Confidence float32

// Common confidence levels.
const (
	ConfidenceLow     Confidence = 0.25
	ConfidenceMedium  Confidence = 0.50
	ConfidenceHigh    Confidence = 0.75
	ConfidenceCertain Confidence = 1.00
)

// Signal is a single labeled observation contributed by a detector.
//
// Source identifies which detector produced the signal, using a colon-separated
// namespace convention (e.g. "header:Server", "html:generator", "cookie:name").
// This namespacing is a convention; the package does not enforce it.
//
// Value is the raw observed string (e.g. the header value, cookie name, or
// extracted token). It is stored as-is; normalization is the caller's
// responsibility.
//
// Confidence reflects how certain the detector is that Value indicates the
// presence of a technology or characteristic.
//
// Signal is a value type. It is copied when passed to AddSignal, so callers
// do not need to retain it after the call.
type Signal struct {
	Source     string
	Value      string
	Confidence Confidence
}

// Fingerprint accumulates observations about a single target over the lifetime
// of a scan. It is safe for concurrent use.
//
// Signals are append-only: once recorded, a signal is never removed. This
// reflects the monotonic nature of evidence gathering — information is only
// added, never retracted, during a scan.
//
// Fingerprint does not own any network or I/O resources; it is a pure data
// container.
type Fingerprint struct {
	// host is the normalized authority (scheme + host + port) of the target.
	// It is set at construction time and never mutated.
	host string

	mu      sync.RWMutex
	signals []Signal
}

// newFingerprint creates a Fingerprint for the given normalized host.
// It is unexported because Fingerprint instances are created and owned by Cache.
func newFingerprint(host string) *Fingerprint {
	return &Fingerprint{host: host}
}

// Host returns the normalized authority string this fingerprint belongs to.
func (f *Fingerprint) Host() string {
	return f.host
}

// AddSignal appends a signal to the fingerprint.
// s is copied; the caller does not need to retain it.
func (f *Fingerprint) AddSignal(s Signal) {
	f.mu.Lock()
	f.signals = append(f.signals, s)
	f.mu.Unlock()
}

// Signals returns a snapshot of all signals collected so far.
// The returned slice is a copy and safe to read without holding any lock.
func (f *Fingerprint) Signals() []Signal {
	f.mu.RLock()
	out := make([]Signal, len(f.signals))
	copy(out, f.signals)
	f.mu.RUnlock()
	return out
}

// SignalCount returns the number of signals recorded without allocating a copy.
func (f *Fingerprint) SignalCount() int {
	f.mu.RLock()
	n := len(f.signals)
	f.mu.RUnlock()
	return n
}
