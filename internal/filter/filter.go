package filter

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
)

// FilterSuite encapsulates status, size, regex, and content-type filters
// to provide unified and optimized matching logic.
type FilterSuite struct {
	MatchStatus  status.Filters
	FilterStatus status.Filters
	MatchSize    size.Filters
	FilterSize   size.Filters

	MatchRegex  []*regexp.Regexp
	FilterRegex []*regexp.Regexp

	MatchContentType  []string
	FilterContentType []string

	// Response Presentation fields
	ShowHeaders bool
	ShowTitle   bool
}

// NewFilterSuite constructs and compiles a FilterSuite.
func NewFilterSuite(
	mc, fc string,
	ms, fs string,
	mr, fr []string,
	mt, ft []string,
) (*FilterSuite, error) {
	suite := &FilterSuite{}

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

// RequiresBody returns true if there is at least one regex filter configured or title extraction is enabled.
func (fs *FilterSuite) RequiresBody() bool {
	return len(fs.MatchRegex) > 0 || len(fs.FilterRegex) > 0 || fs.ShowTitle
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

// MatchBody validates response body against configured match/filter regexes.
func (fs *FilterSuite) MatchBody(body []byte) bool {
	// 1. Regex Matching
	if len(fs.MatchRegex) > 0 {
		matched := false
		for _, rx := range fs.MatchRegex {
			if rx.Match(body) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	// 2. Regex Filtering
	if len(fs.FilterRegex) > 0 {
		for _, rx := range fs.FilterRegex {
			if rx.Match(body) {
				return false
			}
		}
	}

	return true
}
