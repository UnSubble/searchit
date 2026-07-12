package fingerprint

import "strings"

// matchReact evaluates signals for indicators of React.
// Evidence combined:
// - data-reactroot or data-react-html attribute (confidence = 0.95)
// - script src path containing react (confidence = 0.7)
// - root/app container element ID (confidence = 0.15)
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

	p := 1.0
	if hasReactAttr {
		p *= (1.0 - 0.95)
	}
	if hasReactScript {
		p *= (1.0 - 0.70)
	}
	if hasRootId {
		p *= (1.0 - 0.15)
	}

	return Confidence(1.0 - p), true
}
