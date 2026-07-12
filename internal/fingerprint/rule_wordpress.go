package fingerprint

import "strings"

// matchWordPress evaluates signals for indicators of WordPress.
// Evidence combined:
// - meta generator tag (confidence = 0.95)
// - script/link stylesheet containing /wp-content/ or /wp-includes/ (confidence = 0.9)
// - cookie starting with wordpress_ or wp-settings- (confidence = 0.9)
func matchWordPress(signals []Signal) (Confidence, bool) {
	var (
		hasGeneratorMeta bool
		hasWpAssets      bool
		hasWpCookie      bool
	)

	for _, s := range signals {
		// 1. Meta generator check
		if s.Source == "html:meta:name:generator" && strings.Contains(strings.ToLower(s.Value), "wordpress") {
			hasGeneratorMeta = true
		}

		// 2. Resource link/script check
		if (s.Source == "html:link:stylesheet" || s.Source == "html:script") &&
			(strings.Contains(s.Value, "/wp-content/") || strings.Contains(s.Value, "/wp-includes/")) {
			hasWpAssets = true
		}

		// 3. Cookies check
		if s.Source == "cookie" && (strings.HasPrefix(s.Value, "wordpress_") || strings.HasPrefix(s.Value, "wp-settings-")) {
			hasWpCookie = true
		}
	}

	if !hasGeneratorMeta && !hasWpAssets && !hasWpCookie {
		return 0, false
	}

	p := 1.0
	if hasGeneratorMeta {
		p *= (1.0 - 0.95)
	}
	if hasWpAssets {
		p *= (1.0 - 0.90)
	}
	if hasWpCookie {
		p *= (1.0 - 0.90)
	}

	return Confidence(1.0 - p), true
}
