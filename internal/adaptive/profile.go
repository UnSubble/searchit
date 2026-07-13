// Package adaptive provides the infrastructure for technology-specific adaptive
// profiles. Adaptive profiles allow Searchit to adjust scanning behaviour based
// on the technology running on a target.
//
// A profile can be selected automatically via fingerprint detection, or
// explicitly by the user via the --tech flag. When --tech is used, fingerprint
// detection is skipped entirely and the named profile becomes authoritative.
//
// This package owns:
//   - the TechProfile data model
//   - the built-in registry of supported technologies
//   - profile lookup and validation
//
// It does not perform scanning and does not import any scan-engine packages.
package adaptive

// TechProfile describes a technology-specific adaptive scanning profile.
//
// The model is intentionally minimal. Future pull requests will extend it with
// framework-specific hints such as priority paths, common extensions, and
// recursion preferences as real consumers are introduced.
type TechProfile struct {
	// ID is the canonical lowercase identifier for this technology.
	// This is the value the user passes to --tech (e.g. "laravel", "spring").
	ID string

	// DisplayName is the human-readable name shown in output and help text
	// (e.g. "Laravel", "Spring Boot").
	DisplayName string
}
