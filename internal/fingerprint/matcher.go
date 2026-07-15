package fingerprint

// Matcher runs the technology detection rules against a Fingerprint to identify
// active frameworks, servers, or languages.
type Matcher struct{}

// NewMatcher creates a new technology Matcher.
func NewMatcher() *Matcher {
	return &Matcher{}
}

// Match evaluates all registered technology rules against the Fingerprint.
// It returns a slice of matched technologies.
func (m *Matcher) Match(fp *Fingerprint) []Technology {
	signals := fp.Signals()
	if len(signals) == 0 {
		return nil
	}

	var results []Technology

	// Statically call each technology rule.
	// As new rules are added, register them here.
	if matchLaravel(signals) {
		results = append(results, Technology{Name: "Laravel"})
	}
	if matchWordPress(signals) {
		results = append(results, Technology{Name: "WordPress"})
	}
	if matchReact(signals) {
		results = append(results, Technology{Name: "React"})
	}

	return results
}
