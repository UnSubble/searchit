// Package fingerprint provides the data model and storage for per-target
// observations accumulated during a scan.
//
// The fingerprint engine is the single source of truth for information
// learned about a target. Future adaptive scanning components—including
// robots.txt parsing, HTML link discovery, framework detection, extension
// inference, and content-type strategies—should store and retrieve their
// observations through this package rather than each maintaining their own
// per-target state.
//
// # Architectural constraints
//
// This package is deliberately orthogonal. It does not import and must never
// import any other internal Searchit package. It knows nothing about scanning,
// recursion, rendering, output, CLI flags, or queue management. Its sole
// responsibility is owning fingerprint data and its lifecycle.
package fingerprint
