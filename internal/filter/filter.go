package filter

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
)

// Supported regex scope values.
const (
	RegexScopeURL    = "url"
	RegexScopeBody   = "body"
	RegexScopeHeader = "header"
	RegexScopeStatus = "status"
	RegexScopeAll    = "all"
)

// IsValidRegexScope returns true if s is a recognized regex scope.
func IsValidRegexScope(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case RegexScopeURL, RegexScopeBody, RegexScopeHeader, RegexScopeStatus, RegexScopeAll:
		return true
	default:
		return false
	}
}

// FilterSuite encapsulates status, size, regex, and content-type filters
// to provide unified and optimized matching logic.
type FilterSuite struct {
	MatchStatus  status.Filters
	FilterStatus status.Filters
	MatchSize    size.Filters
	FilterSize   size.Filters

	MatchRegex  []*regexp.Regexp
	FilterRegex []*regexp.Regexp
	RegexScope  string

	MatchContentType  []string
	FilterContentType []string

	// Response Presentation fields
	ShowHeaders bool
	ShowTitle   bool
}

// NewFilterSuite constructs and compiles a FilterSuite.
// An optional scope string can be provided (url, body, header, status, all).
// If no scope is provided, it defaults to "body" for backward compatibility with existing tests.
func NewFilterSuite(
	mc, fc string,
	ms, fs string,
	mr, fr []string,
	mt, ft []string,
	scope ...string,
) (*FilterSuite, error) {
	regexScope := RegexScopeBody
	if len(scope) > 0 && scope[0] != "" {
		s := strings.ToLower(strings.TrimSpace(scope[0]))
		if !IsValidRegexScope(s) {
			return nil, fmt.Errorf("invalid regex-scope %q: must be one of url, body, header, status, all", scope[0])
		}
		regexScope = s
	}

	return NewFilterSuiteWithScope(mc, fc, ms, fs, mr, fr, mt, ft, regexScope)
}

// NewFilterSuiteWithScope constructs and compiles a FilterSuite with an explicit regex scope.
func NewFilterSuiteWithScope(
	mc, fc string,
	ms, fs string,
	mr, fr []string,
	mt, ft []string,
	scope string,
) (*FilterSuite, error) {
	suite := &FilterSuite{}

	s := strings.ToLower(strings.TrimSpace(scope))
	if s == "" {
		s = RegexScopeBody
	}
	if !IsValidRegexScope(s) {
		return nil, fmt.Errorf("invalid regex-scope %q: must be one of url, body, header, status, all", scope)
	}
	suite.RegexScope = s

	// 1. Status Codes
	if mc != "" {
		f, err := status.Parse(mc)
		if err != nil {
			return nil, fmt.Errorf("invalid match-status %q: %w", mc, err)
		}
		suite.MatchStatus = f
	}
	if fc != "" {
		f, err := status.Parse(fc)
		if err != nil {
			return nil, fmt.Errorf("invalid filter-status %q: %w", fc, err)
		}
		suite.FilterStatus = f
	}

	// 2. Sizes
	if ms != "" {
		f, err := size.Parse(ms)
		if err != nil {
			return nil, fmt.Errorf("invalid match-size %q: %w", ms, err)
		}
		suite.MatchSize = f
	}
	if fs != "" {
		f, err := size.Parse(fs)
		if err != nil {
			return nil, fmt.Errorf("invalid filter-size %q: %w", fs, err)
		}
		suite.FilterSize = f
	}

	// 3. Regex Matchers
	for _, p := range mr {
		rx, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid match-regex %q: %w", p, err)
		}
		suite.MatchRegex = append(suite.MatchRegex, rx)
	}
	for _, p := range fr {
		rx, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid filter-regex %q: %w", p, err)
		}
		suite.FilterRegex = append(suite.FilterRegex, rx)
	}

	// 4. Content-Types
	for _, t := range mt {
		suite.MatchContentType = append(suite.MatchContentType, strings.ToLower(strings.TrimSpace(t)))
	}
	for _, t := range ft {
		suite.FilterContentType = append(suite.FilterContentType, strings.ToLower(strings.TrimSpace(t)))
	}

	return suite, nil
}

// RequiresBody returns true if response body content is needed for regex matching
// or HTML title extraction. When regex matching only targets URL, headers, or status,
// RequiresBody returns false so unnecessary body reads can be skipped.
func (fs *FilterSuite) RequiresBody() bool {
	if fs.ShowTitle {
		return true
	}
	if len(fs.MatchRegex) > 0 || len(fs.FilterRegex) > 0 {
		switch fs.RegexScope {
		case RegexScopeBody, RegexScopeAll:
			return true
		}
	}
	return false
}

// MatchHeaders validates status code, content size, and Content-Type header.
func (fs *FilterSuite) MatchHeaders(statusCode int, length int64, contentType string) bool {
	// 1. Status Code Matching
	if len(fs.MatchStatus) > 0 {
		if !fs.MatchStatus.Match(statusCode) {
			return false
		}
	}
	// 2. Status Code Filtering
	if len(fs.FilterStatus) > 0 {
		if fs.FilterStatus.Match(statusCode) {
			return false
		}
	}

	// 3. Content-Type Matching
	if len(fs.MatchContentType) > 0 {
		matched := false
		lowerCT := strings.ToLower(contentType)
		for _, t := range fs.MatchContentType {
			if strings.Contains(lowerCT, t) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	// 4. Content-Type Filtering
	if len(fs.FilterContentType) > 0 {
		lowerCT := strings.ToLower(contentType)
		for _, t := range fs.FilterContentType {
			if strings.Contains(lowerCT, t) {
				return false
			}
		}
	}

	// 5. Size Matching
	if len(fs.MatchSize) > 0 {
		if !fs.MatchSize.Match(length) {
			return false
		}
	}
	// 6. Size Filtering
	if len(fs.FilterSize) > 0 {
		if fs.FilterSize.Match(length) {
			return false
		}
	}

	return true
}

// MatchResponse evaluates configured regex filters against the response data according to RegexScope.
// targetURL is the final response URL, statusCode is the final response status,
// headers are the response headers, and body is the response body content.
func (fs *FilterSuite) MatchResponse(targetURL string, statusCode int, headers http.Header, body []byte) bool {
	if len(fs.MatchRegex) == 0 && len(fs.FilterRegex) == 0 {
		return true
	}

	targets := fs.scopedTargets(targetURL, statusCode, headers, body)
	return fs.matchTargets(targets)
}

// MatchBody validates response body against configured match/filter regexes.
// Preserved for backward compatibility.
func (fs *FilterSuite) MatchBody(body []byte) bool {
	return fs.matchTargets([][]byte{body})
}

func (fs *FilterSuite) scopedTargets(targetURL string, statusCode int, headers http.Header, body []byte) [][]byte {
	switch fs.RegexScope {
	case RegexScopeURL:
		return [][]byte{[]byte(targetURL)}
	case RegexScopeBody:
		return [][]byte{body}
	case RegexScopeHeader:
		return [][]byte{formatHeaders(headers)}
	case RegexScopeStatus:
		return [][]byte{[]byte(strconv.Itoa(statusCode))}
	case RegexScopeAll:
		return [][]byte{
			[]byte(targetURL),
			[]byte(strconv.Itoa(statusCode)),
			formatHeaders(headers),
			body,
		}
	default:
		return [][]byte{body}
	}
}

func (fs *FilterSuite) matchTargets(targets [][]byte) bool {
	// 1. Regex Matching (-mr): at least one regex must match at least one target in scope
	if len(fs.MatchRegex) > 0 {
		matched := false
		for _, rx := range fs.MatchRegex {
			for _, target := range targets {
				if rx.Match(target) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}

	// 2. Regex Filtering (-fr): if any regex matches any target in scope, exclude
	if len(fs.FilterRegex) > 0 {
		for _, rx := range fs.FilterRegex {
			for _, target := range targets {
				if rx.Match(target) {
					return false
				}
			}
		}
	}

	return true
}

// formatHeaders writes HTTP headers in sorted, deterministic key-value lines.
func formatHeaders(headers http.Header) []byte {
	if headers == nil {
		return nil
	}
	var sb strings.Builder
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range headers[k] {
			sb.WriteString(k)
			sb.WriteString(": ")
			sb.WriteString(v)
			sb.WriteString("\n")
		}
	}
	return []byte(sb.String())
}
