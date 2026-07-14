package discovery

import "time"

// SignalType categorizes the origin/nature of the observation.
type SignalType string

const (
	SignalHeader      SignalType = "header"
	SignalCookie      SignalType = "cookie"
	SignalHTMLAttr    SignalType = "html:attr"
	SignalHTMLScript  SignalType = "html:script"
	SignalHTMLComment SignalType = "html:comment"
	SignalRobots      SignalType = "robots"
	SignalSitemap     SignalType = "sitemap"
	SignalContentType SignalType = "content-type"
)

// Signal represents an immutable observation gathered during scanning.
type Signal struct {
	Type      SignalType        // Category of the signal
	Source    string            // Label/Name of the source (e.g. "X-Powered-By", "wordpress_logged_in")
	Target    string            // Normalized target host (e.g. "localhost:8080")
	Value     string            // The raw observation value
	Timestamp time.Time         // Time of observation
	Metadata  map[string]string // Optional context keys/values
}
