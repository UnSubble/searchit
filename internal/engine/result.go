package engine

import "net/http"

// Result carries the metadata produced by a successful pipeline pass.
// The body is never stored — only cheap scalar metadata survives.
type Result struct {
	URL         string
	RedirectURL string
	StatusCode  int
	Length      int64 // -1 when Content-Length is absent
	Depth       uint16
	Accepted    bool
	Origin      string
	Err         error

	// Presentation metadata
	Title   string
	Headers http.Header

	// Extracted links for smarter recursive crawling
	Links []string

	// Hash of the response body for wildcard/duplicate detection
	BodyHash uint64
}
