// Package status provides HTTP status code pattern matching and parsing.
// Supported formats: exact (200), wildcard (2xx, 40x), range (100-200),
// and comma-separated combinations. Surrounding whitespace is ignored.
package status

import (
	"fmt"
	"strconv"
	"strings"
)

// Pattern is implemented by all status code matchers.
type Pattern interface {
	Match(code int) bool
	String() string
}

// ExactPattern matches a single HTTP status code.
type ExactPattern struct {
	code int
}

func (p ExactPattern) Match(code int) bool {
	return code == p.code
}

func (p ExactPattern) String() string {
	return strconv.Itoa(p.code)
}

// WildcardPattern matches using a 3-char template where 'x' is a digit wildcard.
// The template is always lower-case (e.g. "2xx", "40x").
type WildcardPattern struct {
	template string
}

func (p WildcardPattern) Match(code int) bool {
	if code < 100 || code > 999 {
		return false
	}
	actual := strconv.Itoa(code)
	for i := 0; i < 3; i++ {
		if p.template[i] != 'x' && p.template[i] != actual[i] {
			return false
		}
	}
	return true
}

func (p WildcardPattern) String() string {
	return p.template
}

// RangePattern matches codes within an inclusive [low, high] range.
type RangePattern struct {
	low  int
	high int
}

func (p RangePattern) Match(code int) bool {
	return code >= p.low && code <= p.high
}

func (p RangePattern) String() string {
	return fmt.Sprintf("%d-%d", p.low, p.high)
}

// Filters is a slice of patterns with OR semantics. Empty filters never match.
type Filters []Pattern

func (f Filters) Match(code int) bool {
	for _, p := range f {
		if p.Match(code) {
			return true
		}
	}
	return false
}

func (f Filters) String() string {
	parts := make([]string, len(f))
	for i, p := range f {
		parts[i] = p.String()
	}
	return strings.Join(parts, ",")
}

// MustParse parses a comma-separated expression into Filters, panicking on failure.
// It is intended for initialization and test helpers.
func MustParse(input string) Filters {
	filters, err := Parse(input)
	if err != nil {
		panic("status: MustParse(" + input + "): " + err.Error())
	}
	return filters
}

// Parse converts a comma-separated expression into Filters.
// Returns an error for any invalid token.
func Parse(input string) (Filters, error) {
	tokens := strings.Split(input, ",")
	filters := make(Filters, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			return nil, fmt.Errorf("status: empty token in expression %q", input)
		}
		p, err := parseToken(token)
		if err != nil {
			return nil, err
		}
		filters = append(filters, p)
	}
	return filters, nil
}

func parseToken(token string) (Pattern, error) {
	if idx := strings.Index(token, "-"); idx > 0 {
		return parseRange(token, idx)
	}
	if strings.HasPrefix(token, "-") {
		return nil, fmt.Errorf("status: expression %q must not begin with '-'", token)
	}
	return parseExactOrWildcard(token)
}

func parseRange(token string, idx int) (Pattern, error) {
	rawLow := strings.TrimSpace(token[:idx])
	rawHigh := strings.TrimSpace(token[idx+1:])

	if rawHigh == "" {
		return nil, fmt.Errorf("status: range %q is missing an upper bound", token)
	}
	if containsWildcard(rawLow) || containsWildcard(rawHigh) {
		return nil, fmt.Errorf("status: wildcard expressions cannot be used as range bounds in %q", token)
	}

	low, err := parseExactCode(rawLow, token)
	if err != nil {
		return nil, err
	}
	high, err := parseExactCode(rawHigh, token)
	if err != nil {
		return nil, err
	}
	if low > high {
		return nil, fmt.Errorf("status: lower bound %d exceeds upper bound %d in %q", low, high, token)
	}
	return RangePattern{low: low, high: high}, nil
}

func parseExactOrWildcard(token string) (Pattern, error) {
	if len(token) != 3 {
		return nil, fmt.Errorf("status: %q must be a 3-character code or wildcard (e.g. 200, 2xx)", token)
	}
	if containsWildcard(token) {
		return parseWildcard(token)
	}
	code, err := parseExactCode(token, token)
	if err != nil {
		return nil, err
	}
	return ExactPattern{code: code}, nil
}

func parseWildcard(token string) (Pattern, error) {
	lower := strings.ToLower(token)
	for i, ch := range lower {
		if ch != 'x' && (ch < '0' || ch > '9') {
			return nil, fmt.Errorf("status: wildcard %q has invalid character %q at position %d", token, string(ch), i)
		}
	}
	if lower[0] == '0' {
		return nil, fmt.Errorf("status: wildcard %q has a leading zero", token)
	}
	return WildcardPattern{template: lower}, nil
}

func parseExactCode(s, context string) (int, error) {
	if len(s) != 3 {
		return 0, fmt.Errorf("status: %q in %q must be exactly 3 digits", s, context)
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("status: %q in %q contains non-digit character %q", s, context, string(ch))
		}
	}
	n, _ := strconv.Atoi(s) // safe: already validated all digits
	if n < 100 || n > 999 {
		return 0, fmt.Errorf("status: %d in %q is out of valid range (100–999)", n, context)
	}
	return n, nil
}

func containsWildcard(s string) bool {
	return strings.ContainsAny(s, "xX")
}
