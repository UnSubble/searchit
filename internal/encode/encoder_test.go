package encode

import (
	"strings"
	"testing"
)

func TestEncoder_All(t *testing.T) {
	// Noop
	noop, err := NewEncoder("")
	if err != nil || noop.Name() != "" || noop.IsWordlistScoped() || noop.Encode("hello") != "hello" {
		t.Errorf("unexpected noop behavior: %v", noop)
	}

	// Base64
	b64, err := NewEncoder("base64")
	if err != nil || b64.Name() != "base64" || b64.IsWordlistScoped() || b64.Encode("admin") != "YWRtaW4=" {
		t.Errorf("unexpected base64 behavior: %v", b64)
	}

	// URL
	urlEnc, err := NewEncoder("url")
	if err != nil || urlEnc.Name() != "url" || urlEnc.IsWordlistScoped() || urlEnc.Encode("a/b c") != "a%2Fb%20c" {
		t.Errorf("unexpected url behavior: %v", urlEnc)
	}

	// DoubleURL
	dblURL, err := NewEncoder("doubleurl")
	if err != nil || dblURL.Name() != "doubleurl" || dblURL.IsWordlistScoped() || dblURL.Encode("a/b") != "a%252Fb" {
		t.Errorf("unexpected doubleurl behavior: %v", dblURL)
	}

	// Wordlist variants
	wlB64, err := NewEncoder("wl-base64")
	if err != nil || !wlB64.IsWordlistScoped() || wlB64.Encode("admin") != "YWRtaW4=" {
		t.Errorf("unexpected wl-base64 behavior: %v", wlB64)
	}

	wlURL, err := NewEncoder("wl-url")
	if err != nil || !wlURL.IsWordlistScoped() || wlURL.Encode("a/b") != "a%2Fb" {
		t.Errorf("unexpected wl-url behavior: %v", wlURL)
	}

	wlDbl, err := NewEncoder("wl-doubleurl")
	if err != nil || !wlDbl.IsWordlistScoped() || wlDbl.Encode("a/b") != "a%252Fb" {
		t.Errorf("unexpected wl-doubleurl behavior: %v", wlDbl)
	}

	// Invalid
	if _, err := NewEncoder("invalid"); err == nil || !strings.Contains(err.Error(), "invalid encoding") {
		t.Errorf("expected error for invalid encoding, got %v", err)
	}
}
