package wildcard_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/wildcard"
)

func TestWildcardDetector(t *testing.T) {
	d := wildcard.NewDetector()
	host := "example.com"

	// 1. Send 15 normal responses and 5 wildcard responses. No wildcard should be detected yet.
	for i := 0; i < 15; i++ {
		sig := wildcard.Signature{StatusCode: 404, BodyHash: uint64(i), BodySize: int64(100 + i)}
		_, active := d.Add(host, 1, sig)
		if active {
			t.Fatalf("wildcard active prematurely")
		}
	}

	// 2. Add more wildcard responses until we hit 20 samples.
	wildcardSig := wildcard.Signature{StatusCode: 200, BodyHash: 0xDEADBEEF, BodySize: 500}
	for i := 0; i < 19; i++ {
		d.Add(host, 1, wildcardSig)
	}

	// The 20th wildcard sample should trigger detection because 19/34 is not >80%.
	// Wait, let's make sure we have >80% wildcard signatures in history.
	// To have 80% of 20 samples, we need at least 16 matching wildcard signatures.
	d2 := wildcard.NewDetector()
	for i := 0; i < 4; i++ {
		d2.Add(host, 1, wildcard.Signature{StatusCode: 404, BodyHash: uint64(i), BodySize: 100})
	}
	for i := 0; i < 16; i++ {
		_, active := d2.Add(host, 1, wildcardSig)
		if i < 15 && active {
			t.Fatalf("wildcard active prematurely at step %d", i)
		}
		if i == 15 {
			if !active {
				t.Fatalf("expected wildcard to be active at step 15")
			}
		}
	}

	// Test IsWildcard
	if !d2.IsWildcard(host, 1, wildcardSig) {
		t.Errorf("expected IsWildcard to return true for wildcardSig")
	}

	// Different host should not trigger wildcard detection if it hasn't had its own history
	otherHost := "other.com"
	if d2.IsWildcard(otherHost, 1, wildcardSig) {
		t.Errorf("expected IsWildcard to return false for different host without history")
	}

	nonWildcardSig := wildcard.Signature{StatusCode: 200, BodyHash: 0x12345, BodySize: 123}
	if d2.IsWildcard(host, 1, nonWildcardSig) {
		t.Errorf("expected IsWildcard to return false for non-wildcard signature")
	}
}
