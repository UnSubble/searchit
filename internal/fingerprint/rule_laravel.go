package fingerprint

import "strings"

// matchLaravel evaluates signals for indicators of the Laravel framework.
// Evidence combined:
// - laravel_session cookie (confidence = 0.9)
// - livewire.js script source (confidence = 0.8)
// - php X-Powered-By/Server header (confidence = 0.15)
func matchLaravel(signals []Signal) (Confidence, bool) {
	var (
		hasLaravelSession bool
		hasLivewireScript bool
		hasPHPHeader      bool
	)

	for _, s := range signals {
		// 1. Cookie match
		if s.Source == "cookie" && s.Value == "laravel_session" {
			hasLaravelSession = true
		}

		// 2. HTML Livewire script check
		if s.Source == "html:script" && strings.Contains(strings.ToLower(s.Value), "livewire") {
			hasLivewireScript = true
		}

		// 3. Supporting PHP headers check
		if s.Source == "header:X-Powered-By" && strings.Contains(strings.ToLower(s.Value), "php") {
			hasPHPHeader = true
		}
	}

	if !hasLaravelSession && !hasLivewireScript && !hasPHPHeader {
		return 0, false
	}

	// Calculate combined probability: 1 - (1 - c1)(1 - c2)...
	p := 1.0
	if hasLaravelSession {
		p *= (1.0 - 0.90)
	}
	if hasLivewireScript {
		p *= (1.0 - 0.80)
	}
	if hasPHPHeader {
		p *= (1.0 - 0.15)
	}

	return Confidence(1.0 - p), true
}
