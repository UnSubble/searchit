package engine

// Result carries the metadata produced by a successful pipeline pass.
// The body is never stored — only cheap scalar metadata survives.
type Result struct {
	URL        string
	StatusCode int
	Length     int64 // -1 when Content-Length is absent
	Depth      uint16
	Accepted   bool
	Origin     string
	Err        error
}
