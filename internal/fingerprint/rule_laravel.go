package fingerprint

import "strings"

// matchLaravel evaluates signals for indicators of the Laravel framework.
func matchLaravel(signals []Signal) (Confidence, bool) {
	var (
		hasLaravelSession bool
		hasLivewireScript bool
		hasPHPHeader      bool
		hasASPNetHeader   bool
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

		// 3. Supporting PHP and ASP.NET headers check
		if s.Source == "header:X-Powered-By" {
			valLower := strings.ToLower(s.Value)
			if strings.Contains(valLower, "php") {
				hasPHPHeader = true
			}
			if strings.Contains(valLower, "asp.net") {
				hasASPNetHeader = true
			}
		}
	}

	// Exclusions
	if hasASPNetHeader {
		return 0, false
	}

	// Deterministic scoring hierarchy
	switch {
	case hasLaravelSession:
		return Confidence(1.0), true
	case hasLivewireScript && hasPHPHeader:
		return Confidence(0.85), true
	case hasLivewireScript:
		return Confidence(0.70), true
	case hasPHPHeader:
		return Confidence(0.15), true
	default:
		return 0, false
	}
}
