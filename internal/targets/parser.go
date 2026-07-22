package targets

import (
	"fmt"
	"strings"

	"github.com/unsubble/searchit/internal/requesttemplate"
)

// ParseOptions contains the parameters required to parse targets from flags.
type ParseOptions struct {
	URL         string // from -u
	URLFile     string // from --url-file
	RequestFile string // from --request
}

// Parse extracts a slice of Target objects based on the provided options.
func Parse(opts ParseOptions) ([]Target, error) {
	var results []Target
	var baseTargets []string

	// 1. Process -u (comma-separated support)
	if opts.URL != "" {
		for _, val := range strings.Split(opts.URL, ",") {
			t := strings.TrimSpace(val)
			if t != "" {
				baseTargets = append(baseTargets, t)
			}
		}
	}

	// 2. Process --url-file
	if opts.URLFile != "" {
		fileTargets, err := ReadFile(opts.URLFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read url file: %w", err)
		}
		baseTargets = append(baseTargets, fileTargets...)
	}

	// 3. If there is a request template, we apply it to all baseTargets or extract it alone.
	if opts.RequestFile != "" {
		var baseTarget string
		if len(baseTargets) > 0 {
			baseTarget = baseTargets[0]
		}

		tmpl, err := requesttemplate.ParseFile(opts.RequestFile, baseTarget)
		if err != nil {
			return nil, fmt.Errorf("failed to parse request template: %w", err)
		}

		cookies := requesttemplate.ExtractCookiesFromHeaders(tmpl.Headers)
		var cookieStr string
		if len(cookies) > 0 {
			var cookiePairs []string
			for _, c := range cookies {
				cookiePairs = append(cookiePairs, c.String())
			}
			cookieStr = strings.Join(cookiePairs, "; ")
		}

		var headers []string
		for k, values := range tmpl.Headers {
			for _, v := range values {
				headers = append(headers, fmt.Sprintf("%s: %s", k, v))
			}
		}

		// A template intrinsically defines one core target URL.
		results = append(results, Target{
			URL:     tmpl.URL,
			Method:  tmpl.Method,
			Body:    string(tmpl.Body),
			Headers: headers,
			Cookies: cookieStr,
		})
	} else {
		// Normal URL processing
		for _, t := range baseTargets {
			results = append(results, Target{
				URL: t,
				// Method, Body, Headers will be defaulted by Config
			})
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("at least one target is required")
	}

	// Number the targets sequentially
	for i := range results {
		results[i].ID = i + 1
	}

	return results, nil
}
