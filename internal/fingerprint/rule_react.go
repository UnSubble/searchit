package fingerprint

import "strings"

// matchReact evaluates signals for indicators of React.
func matchReact(signals []Signal) (Confidence, bool) {
	var (
		hasReactAttr   bool
		hasReactScript bool
		hasRootId      bool
	)

	for _, s := range signals {
		// 1. Attribute match
		if s.Source == "html:attr:data-reactroot" || s.Source == "html:attr:data-react-html" {
			hasReactAttr = true
		}

		// 2. Script match
		if s.Source == "html:script" {
			lowerScript := strings.ToLower(s.Value)
			if strings.Contains(lowerScript, "/react") || strings.Contains(lowerScript, "react.") {
				hasReactScript = true
			}
		}

		// 3. Mount ID checks
		if s.Source == "html:id" {
			lowerId := strings.ToLower(s.Value)
			if lowerId == "root" || lowerId == "app" {
				hasRootId = true
			}
		}
	}

	if !hasReactAttr && !hasReactScript && !hasRootId {
		return 0, false
	}

	// Deterministic scoring hierarchy
	switch {
	case hasReactAttr && hasReactScript:
		// React attributes and script assets present together yields certainty
		return Confidence(1.0), true
	case hasReactAttr:
		return Confidence(0.95), true
	case hasReactScript:
		return Confidence(0.70), true
	case hasRootId:
		return Confidence(0.15), true
	default:
		return 0, false
	}
}
