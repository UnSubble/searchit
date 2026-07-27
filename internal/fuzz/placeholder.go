package fuzz

import (
	"net/http"
	"strings"
)

// RequestTemplate represents the uncompiled components of a fuzz request.
// It encapsulates all locations where a placeholder is legally allowed to exist.
type RequestTemplate struct {
	URL     string
	Method  string
	Body    string
	Headers http.Header
	Cookie  string
}

// SupportedPlaceholders defines the complete list of valid fuzz placeholders.
var SupportedPlaceholders = []string{"FUZZ", "FOO", "BAR", "BUZZ"}

// FindPlaceholders inspects a merged RequestTemplate and returns a slice
// containing all supported placeholders that are present within the request.
func FindPlaceholders(req RequestTemplate) []string {
	var found []string

	for _, p := range SupportedPlaceholders {
		if hasPlaceholder(req, p) {
			found = append(found, p)
		}
	}

	return found
}

// hasPlaceholder checks if a specific placeholder exists anywhere within the request template.
func hasPlaceholder(req RequestTemplate, placeholder string) bool {
	if strings.Contains(req.URL, placeholder) {
		return true
	}
	if strings.Contains(req.Body, placeholder) {
		return true
	}
	if strings.Contains(req.Cookie, placeholder) {
		return true
	}
	// Note: We don't typically fuzz the method, but we check everywhere just to be consistent.
	if strings.Contains(req.Method, placeholder) {
		return true
	}

	for k, vals := range req.Headers {
		if strings.Contains(k, placeholder) {
			return true
		}
		for _, v := range vals {
			if strings.Contains(v, placeholder) {
				return true
			}
		}
	}

	return false
}
