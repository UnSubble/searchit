package fingerprint

// Technology represents a detected framework, platform, server, or language.
type Technology struct {
	Name       string     // Name of the technology (e.g. "Laravel", "WordPress")
	Confidence Confidence // Aggregated confidence score in range [0, 1]
}
