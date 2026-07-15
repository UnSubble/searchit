package fingerprint

import "strings"

// matchWordPress evaluates signals for indicators of WordPress.
// Returns true if any positive indicator is present and no exclusion applies.
func matchWordPress(signals []Signal) bool {
	var (
		hasGeneratorMeta bool
		hasWpAssets      bool
		hasWpCookie      bool
		hasExcludeHeader bool
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

		// 4. Check negative exclusions
		if s.Source == "header:X-Powered-By" {
			valLower := strings.ToLower(s.Value)
			if strings.Contains(valLower, "asp.net") || strings.Contains(valLower, "node.js") {
				hasExcludeHeader = true
			}
		}
	}

	// Exclusions
	if hasExcludeHeader {
		return false
	}

	return hasGeneratorMeta || hasWpAssets || hasWpCookie
}
