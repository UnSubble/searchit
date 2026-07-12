package fingerprint

import (
	"strings"
)

var usefulHeaders = []string{
	"Server",
	"X-Powered-By",
	"X-Generator",
	"X-AspNet-Version",
	"X-Drupal-Cache",
	"Via",
	"Content-Type",
	"X-SharePointHealthScore",
	"X-CMS",
	"X-Framework",
	"X-Redirect-By",
	"X-Version",
}

var usefulHeadersMap = make(map[string]string)

func init() {
	for _, h := range usefulHeaders {
		usefulHeadersMap[strings.ToLower(h)] = h
	}
}

// detectHeaders inspects HTTP response headers, extracts key fingerprinting
// evidence, and records signals into the target Fingerprint.
func detectHeaders(ctx *Context, fp *Fingerprint) {
	if ctx.Header == nil {
		return
	}

	for rawKey, values := range ctx.Header {
		lowerKey := strings.ToLower(rawKey)

		// 1. Process standard useful headers
		if displayKey, ok := usefulHeadersMap[lowerKey]; ok {
			for _, val := range values {
				val = strings.TrimSpace(val)
				if val == "" {
					continue
				}
				fp.AddSignal(Signal{
					Source:     "header:" + displayKey,
					Value:      val,
					Confidence: Confidence(1.0),
				})
			}
		}

		// 2. Process Set-Cookie headers
		if lowerKey == "set-cookie" {
			for _, val := range values {
				cookieName := parseCookieName(val)
				if cookieName != "" {
					fp.AddSignal(Signal{
						Source:     "cookie",
						Value:      cookieName,
						Confidence: Confidence(1.0),
					})
				}
			}
		}
	}
}

// parseCookieName extracts the cookie name from a Set-Cookie value string.
// Set-Cookie values typically follow the RFC 6265 format:
// "CookieName=CookieValue; Option1=Val1; Option2..."
func parseCookieName(val string) string {
	parts := strings.SplitN(val, ";", 2)
	namePart := strings.TrimSpace(parts[0])
	nameParts := strings.SplitN(namePart, "=", 2)
	return strings.TrimSpace(nameParts[0])
}
