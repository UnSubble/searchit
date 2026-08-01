package fuzz

import (
	"net/http"

	"github.com/unsubble/searchit/internal/engine"
)

// RequestDTO is the unit of work representing a fully rendered request configuration.
type RequestDTO struct {
	URL       string
	Method    string
	Body      string
	Headers   map[string][]string
	Cookies   []string
	UserData  any
	IsProbing bool
	FuzzData  *engine.FuzzData
}

// Result carries metadata produced by a fuzzed request execution.
type Result struct {
	URL         string
	RedirectURL string
	StatusCode  int
	Length      int64
	Accepted    bool
	Err         error
	UserData    any

	Title    string
	Headers  http.Header
	FuzzData *engine.FuzzData
}
