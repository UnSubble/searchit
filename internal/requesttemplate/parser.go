package requesttemplate

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// RequestTemplate represents the parsed raw HTTP request.
type RequestTemplate struct {
	Method  string
	URL     string // Resolves to a full URL, e.g. "http://host/path" or containing placeholders
	Headers http.Header
	Body    []byte
}

// Parse parses a raw HTTP template content.
func Parse(content []byte, baseURLStr string) (*RequestTemplate, error) {
	// First split headers block and body block
	var headersBlock []byte
	var bodyBlock []byte

	// Look for standard double-newline separators
	idx := bytes.Index(content, []byte("\r\n\r\n"))
	if idx != -1 {
		headersBlock = content[:idx]
		bodyBlock = content[idx+4:]
	} else {
		idx = bytes.Index(content, []byte("\n\n"))
		if idx != -1 {
			headersBlock = content[:idx]
			bodyBlock = content[idx+2:]
		} else {
			// Entire file is headers block
			headersBlock = content
		}
	}

	reader := bufio.NewReader(bytes.NewReader(headersBlock))

	// Read request line
	reqLine, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read request line: %w", err)
	}
	reqLine = strings.TrimSpace(reqLine)
	if reqLine == "" {
		return nil, fmt.Errorf("malformed HTTP template: empty request line")
	}

	parts := strings.Fields(reqLine)
	if len(parts) == 0 {
		return nil, fmt.Errorf("malformed HTTP template: empty request line")
	}

	method := "GET"
	var rawPath string

	if len(parts) >= 2 {
		method = strings.ToUpper(parts[0])
		rawPath = parts[1]
	} else {
		// Only path specified
		rawPath = parts[0]
	}

	// Parse subsequent headers
	headers := make(http.Header)
	var hostHeader string

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read header line: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			if err == io.EOF {
				break
			}
			continue
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx <= 0 {
			// Ignore malformed header line or treat as error? Let's ignore.
			if err == io.EOF {
				break
			}
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		val := strings.TrimSpace(line[colonIdx+1:])

		if strings.EqualFold(key, "Host") {
			hostHeader = val
		}

		headers.Add(key, val)

		if err == io.EOF {
			break
		}
	}

	// Resolve the final URL.
	finalURL, err := ResolveURL(baseURLStr, hostHeader, rawPath)
	if err != nil {
		return nil, err
	}

	return &RequestTemplate{
		Method:  method,
		URL:     finalURL,
		Headers: headers,
		Body:    bodyBlock,
	}, nil
}

// ParseFile parses a raw HTTP template from a file path.
func ParseFile(path string, baseURLStr string) (*RequestTemplate, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read request template file: %w", err)
	}
	return Parse(content, baseURLStr)
}

// ResolveURL resolves template path and host/base URLs.
func ResolveURL(baseURLStr string, hostHeader string, rawPath string) (string, error) {
	// If base URL is provided, we use it to resolve references.
	if baseURLStr != "" {
		if hasPlaceholders(rawPath) {
			return resolvePlaceholderURL(baseURLStr, rawPath), nil
		}

		base, err := url.Parse(baseURLStr)
		if err != nil {
			return "", fmt.Errorf("invalid base URL %q: %w", baseURLStr, err)
		}

		// If rawPath is empty/slash, or relative, resolve reference.
		ref, err := url.Parse(rawPath)
		if err != nil {
			return "", fmt.Errorf("invalid request path %q: %w", rawPath, err)
		}

		resolved := base.ResolveReference(ref)
		return resolved.String(), nil
	}

	// No base URL provided, must determine host from template.
	if hostHeader == "" {
		// Try to see if rawPath is already a full URL.
		if strings.HasPrefix(rawPath, "http://") || strings.HasPrefix(rawPath, "https://") {
			return rawPath, nil
		}
		return "", fmt.Errorf("missing Host header or base URL in template")
	}

	// Host header is available, construct http://host/path
	scheme := "http"

	// Format host and path cleanly
	host := hostHeader
	path := rawPath
	if !strings.HasPrefix(path, "/") && path != "" {
		path = "/" + path
	}

	return fmt.Sprintf("%s://%s%s", scheme, host, path), nil
}

func hasPlaceholders(s string) bool {
	return strings.Contains(s, "FUZZ") || strings.Contains(s, "FOO") || strings.Contains(s, "BAR") || strings.Contains(s, "BUZZ")
}

func resolvePlaceholderURL(base, path string) string {
	base = strings.TrimSuffix(base, "/")
	if strings.HasPrefix(path, "/") {
		return base + path
	}
	return base + "/" + path
}

// ExtractCookiesFromHeaders parses Cookie header values and removes the Cookie header from the map.
func ExtractCookiesFromHeaders(headers http.Header) []*http.Cookie {
	cookieStr := headers.Get("Cookie")
	if cookieStr == "" {
		return nil
	}
	headers.Del("Cookie")

	header := http.Header{"Cookie": []string{cookieStr}}
	req := &http.Request{Header: header}
	return req.Cookies()
}
