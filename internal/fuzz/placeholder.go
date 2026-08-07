package fuzz

import (
	"fmt"
	"net/http"
	"sort"
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
var SupportedPlaceholders = []string{"FUZZ", "FOO", "BAR", "BAZ", "BUZZ"}

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

// DetectPlaceholderLocations returns all locations where a placeholder appears
// within a RequestTemplate (e.g. "URL", "Query parameter", "Header: <name>", "Body", "Cookie", "Method").
// Header keys are evaluated in deterministic alphabetical order.
func DetectPlaceholderLocations(req RequestTemplate, placeholder string) []string {
	var locations []string

	if req.URL != "" {
		parts := strings.SplitN(req.URL, "?", 2)
		urlPart := parts[0]
		if strings.Contains(urlPart, placeholder) {
			locations = append(locations, "URL")
		}
		if len(parts) > 1 {
			queryPart := parts[1]
			if strings.Contains(queryPart, placeholder) {
				locations = append(locations, "Query parameter")
			}
		}
	}

	if len(req.Headers) > 0 {
		var headerKeys []string
		for k := range req.Headers {
			headerKeys = append(headerKeys, k)
		}
		sort.Strings(headerKeys)

		for _, k := range headerKeys {
			vals := req.Headers[k]
			found := false
			if strings.Contains(k, placeholder) {
				found = true
			}
			for _, v := range vals {
				if strings.Contains(v, placeholder) {
					found = true
					break
				}
			}
			if found {
				locations = append(locations, fmt.Sprintf("Header: %s", k))
			}
		}
	}

	if req.Body != "" && strings.Contains(req.Body, placeholder) {
		locations = append(locations, "Body")
	}

	if req.Cookie != "" && strings.Contains(req.Cookie, placeholder) {
		locations = append(locations, "Cookie")
	}

	if req.Method != "" && strings.Contains(req.Method, placeholder) {
		locations = append(locations, "Method")
	}

	return locations
}

// GetPlaceholderLocations returns the detected locations joined by commas, or "None" if none.
func GetPlaceholderLocations(req RequestTemplate, placeholder string) string {
	locs := DetectPlaceholderLocations(req, placeholder)
	if len(locs) == 0 {
		return "None"
	}
	return strings.Join(locs, ", ")
}
