package fingerprint

import "net/http"

// Context contains the response details and body content needed by detectors
// to extract signals. It is allocated once per processed request.
type Context struct {
	Host       string      // Normalized scheme + host + port of the target
	Path       string      // URL path scanned (e.g., "/index.html")
	StatusCode int         // HTTP Status Code
	Header     http.Header // HTTP Response Headers
	Body       []byte      // Response Body segment (up to bodyReadLimit)
}
