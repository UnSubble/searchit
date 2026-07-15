package fingerprint

import "strings"

// matchReact evaluates signals for indicators of React.
// Returns true if any positive indicator is present.
func matchReact(signals []Signal) bool {
	for _, s := range signals {
		// 1. Attribute match
		if s.Source == "html:attr:data-reactroot" || s.Source == "html:attr:data-react-html" {
			return true
		}

		// 2. Script match
		if s.Source == "html:script" {
			lowerScript := strings.ToLower(s.Value)
			if strings.Contains(lowerScript, "/react") || strings.Contains(lowerScript, "react.") {
				return true
			}
		}

		// 3. Mount ID checks
		if s.Source == "html:id" {
			lowerId := strings.ToLower(s.Value)
			if lowerId == "root" || lowerId == "app" {
				return true
			}
		}
	}

	return false
}
