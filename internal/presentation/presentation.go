package presentation

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// Path truncates a filesystem path from the left by replacing intermediate
// directory segments with ".../" until the path fits within maxLen.
// It prioritizes preserving the filename and the immediate parent directory.
func Path(p string, maxLen int) string {
	if len(p) <= maxLen || maxLen <= 3 {
		return p
	}

	p = filepath.Clean(p)
	normalized := strings.ReplaceAll(p, "\\", "/")
	segments := strings.Split(normalized, "/")

	if len(segments) <= 2 {
		return truncateMiddle(p, maxLen)
	}

	filename := segments[len(segments)-1]

	var parent string
	if len(segments) > 1 {
		parent = segments[len(segments)-2]
	}

	startIndex := 0
	if segments[0] == "" || strings.HasSuffix(segments[0], ":") {
		startIndex = 1
	}

	for i := startIndex; i < len(segments)-2; i++ {
		var test []string
		if startIndex > 0 && segments[0] != "" {
			test = append(test, segments[0])
		}
		test = append(test, "...")
		test = append(test, segments[i+1:]...)

		candidate := strings.Join(test, "/")
		if len(candidate) <= maxLen {
			return filepath.FromSlash(candidate)
		}
	}

	var minimal []string
	minimal = append(minimal, "...")
	if parent != "" {
		minimal = append(minimal, parent)
	}
	minimal = append(minimal, filename)
	candidate := strings.Join(minimal, "/")
	if len(candidate) > maxLen {
		if len(filename) > maxLen-4 {
			return ".../" + filename[len(filename)-(maxLen-4):]
		}
		return candidate
	}

	return filepath.FromSlash(candidate)
}

// URL truncates a URL from the middle by collapsing intermediate path segments
// or query string values, preserving the host, resource name, and query keys.
func URL(u string, maxLen int) string {
	if len(u) <= maxLen || maxLen <= 3 {
		return u
	}

	parsed, err := url.Parse(u)
	if err != nil {
		return truncateMiddle(u, maxLen)
	}

	segments := strings.Split(parsed.Path, "/")
	if len(segments) > 3 {
		for i := len(segments) - 2; i > 1; i-- {
			var test []string
			test = append(test, segments[:i]...)
			test = append(test, "...")
			test = append(test, segments[len(segments)-1])

			parsed.Path = strings.Join(test, "/")
			if len(parsed.String()) <= maxLen {
				return parsed.String()
			}
		}
		if len(segments) > 2 {
			parsed.Path = segments[0] + "/.../" + segments[len(segments)-1]
		}
	}

	if len(parsed.String()) > maxLen && parsed.RawQuery != "" {
		q := parsed.Query()
		for k, v := range q {
			if len(v) > 0 && len(v[0]) > 5 {
				q.Set(k, v[0][:3]+"...")
			}
		}
		parsed.RawQuery = q.Encode()
	}

	res := parsed.String()
	if len(res) > maxLen {
		return truncateMiddle(res, maxLen)
	}
	return res
}

// RelativeURL strips the scheme and host from reqURL if they match targetURL.
func RelativeURL(targetURL, reqURL string) string {
	tgt, err1 := url.Parse(targetURL)
	req, err2 := url.Parse(reqURL)
	if err1 != nil || err2 != nil {
		return reqURL
	}

	if tgt.Host == req.Host && tgt.Scheme == req.Scheme {
		res := req.Path
		if req.RawQuery != "" {
			res += "?" + req.RawQuery
		}
		if req.Fragment != "" {
			res += "#" + req.Fragment
		}
		if res == "" {
			return "/"
		}
		return res
	}

	return reqURL
}

// Redirect formats a redirect, omitting the host for internal redirects.
func Redirect(targetURL, source, dest string) string {
	srcFmt := RelativeURL(targetURL, source)
	dstFmt := RelativeURL(targetURL, dest)
	return fmt.Sprintf("%s \u2192 %s", srcFmt, dstFmt)
}

// Token compacts long strings like JWTs, Headers, or Cookies.
func Token(key, payload string, maxLen int) string {
	combined := key + payload
	if len(combined) <= maxLen || maxLen <= 3 {
		return combined
	}

	allowedPayload := maxLen - len(key) - 3 // 3 for "..."
	if allowedPayload <= 0 {
		return truncateMiddle(combined, maxLen)
	}

	headLen := allowedPayload / 2
	tailLen := allowedPayload - headLen

	return key + payload[:headLen] + "..." + payload[len(payload)-tailLen:]
}

// Error strips common Go noise from error messages.
func Error(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "Get \"") || strings.HasPrefix(msg, "Post \"") {
		idx := strings.Index(msg, "\": ")
		if idx != -1 {
			msg = msg[idx+3:]
		}
	}
	return msg
}

// Size returns a human-readable byte size.
func Size(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// Duration returns a human-readable duration (e.g. 01:23:04).
func Duration(d time.Duration) string {
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// Number returns a human-readable number.
func Number(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000.0)
	}
	return fmt.Sprintf("%.1fm", float64(n)/1000000.0)
}

func truncateMiddle(s string, max int) string {
	if len(s) <= max || max <= 3 {
		return s
	}
	head := (max - 3) / 2
	tail := max - 3 - head
	return s[:head] + "..." + s[len(s)-tail:]
}
