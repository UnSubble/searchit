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

var usefulHeadersLower = []string{
	"server",
	"x-powered-by",
	"x-generator",
	"x-aspnet-version",
	"x-drupal-cache",
	"via",
	"content-type",
	"x-sharepointhealthscore",
	"x-cms",
	"x-framework",
	"x-redirect-by",
	"x-version",
}

// detectHeaders inspects HTTP response headers, extracts key fingerprinting
// evidence, and records signals into the target Fingerprint.
func detectHeaders(ctx *Context, fp *Fingerprint) {
	if ctx.Header == nil {
		return
	}

	// 1. Direct lookup for standard useful headers (canonical and lowercased fallback)
	for i, h := range usefulHeaders {
		values, ok := ctx.Header[h]
		if !ok {
			values, ok = map[string][]string(ctx.Header)[usefulHeadersLower[i]]
		}
		if ok {
			for _, val := range values {
				val = strings.TrimSpace(val)
				if val == "" {
					continue
				}
				fp.AddSignal(Signal{
					Source: "header:" + h,
					Value:  val,
				})
			}
		}
	}

	// 2. Direct lookup for Set-Cookie header
	setCookieValues, ok := ctx.Header["Set-Cookie"]
	if !ok {
		setCookieValues, ok = map[string][]string(ctx.Header)["set-cookie"]
	}
	if ok {
		for _, val := range setCookieValues {
			cookieName := parseCookieName(val)
			if cookieName != "" {
				fp.AddSignal(Signal{
					Source: "cookie",
					Value:  cookieName,
				})
			}
		}
	}
}

func isReservedCookieAttr(name string) bool {
	switch len(name) {
	case 4:
		return strings.EqualFold(name, "path")
	case 6:
		return strings.EqualFold(name, "secure") || strings.EqualFold(name, "domain")
	case 7:
		return strings.EqualFold(name, "expires") || strings.EqualFold(name, "max-age")
	case 8:
		return strings.EqualFold(name, "httponly") || strings.EqualFold(name, "samesite")
	}
	return false
}

func parseCookieName(val string) string {
	idx := strings.IndexByte(val, ';')
	namePart := val
	if idx != -1 {
		namePart = val[:idx]
	}
	namePart = strings.TrimSpace(namePart)

	eqIdx := strings.IndexByte(namePart, '=')
	var name string
	if eqIdx != -1 {
		name = strings.TrimSpace(namePart[:eqIdx])
	} else {
		name = strings.TrimSpace(namePart)
	}

	if isReservedCookieAttr(name) {
		return ""
	}
	return name
}
