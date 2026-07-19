package fuzz

import "net/http"

// Job is the unit of work representing a single fuzzed request parameters configuration.
type Job struct {
	URL      string
	Method   string
	Body     []byte
	Headers  http.Header
	Cookies  []*http.Cookie
	UserData any
}

// Result carries metadata produced by a fuzzed request execution.
type Result struct {
	URL        string
	StatusCode int
	Length     int64
	Accepted   bool
	Err        error
	UserData   any

	// Presentation metadata
	Title   string
	Headers http.Header
}
