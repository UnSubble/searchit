// Package size provides content-length pattern matching and parsing.
// Supported formats: exact (123), range (100-200), and comma-separated combinations.
package size

import (
	"fmt"
	"strconv"
	"strings"
)

// Pattern is implemented by all size matchers.
type Pattern interface {
	Match(val int64) bool
	String() string
}

// ExactPattern matches a single exact content size in bytes.
type ExactPattern struct {
	val int64
}

func (p ExactPattern) Match(val int64) bool {
	return val == p.val
}

func (p ExactPattern) String() string {
	return strconv.FormatInt(p.val, 10)
}

// RangePattern matches content sizes within an inclusive range [low, high].
type RangePattern struct {
	low  int64
	high int64
}

func (p RangePattern) Match(val int64) bool {
	return val >= p.low && val <= p.high
}

func (p RangePattern) String() string {
	return fmt.Sprintf("%d-%d", p.low, p.high)
}

// Filters is a collection of patterns with OR matching semantics.
type Filters []Pattern

func (f Filters) Match(val int64) bool {
	for _, p := range f {
		if p.Match(val) {
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

// MustParse parses a comma-separated size expression, panicking on failure.
// It is intended for initialization and test helpers.
func MustParse(input string) Filters {
	f, err := Parse(input)
	if err != nil {
		panic("size: MustParse(" + input + "): " + err.Error())
	}
	return f
}

// Parse parses a comma-separated size expression into Filters.
func Parse(input string) (Filters, error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil
	}
	tokens := strings.Split(input, ",")
	filters := make(Filters, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			return nil, fmt.Errorf("size: empty token in expression %q", input)
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
		return nil, fmt.Errorf("size: expression %q must not begin with '-'", token)
	}
	val, err := parseExactValue(token)
	if err != nil {
		return nil, err
	}
	return ExactPattern{val: val}, nil
}

func parseRange(token string, idx int) (Pattern, error) {
	rawLow := strings.TrimSpace(token[:idx])
	rawHigh := strings.TrimSpace(token[idx+1:])

	if rawHigh == "" {
		return nil, fmt.Errorf("size: range %q is missing an upper bound", token)
	}

	low, err := parseExactValue(rawLow)
	if err != nil {
		return nil, err
	}
	high, err := parseExactValue(rawHigh)
	if err != nil {
		return nil, err
	}
	if low > high {
		return nil, fmt.Errorf("size: lower bound %d exceeds upper bound %d in %q", low, high, token)
	}
	return RangePattern{low: low, high: high}, nil
}

func parseExactValue(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("size: empty value")
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("size: %q contains non-digit character %q", s, string(ch))
		}
	}
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("size: invalid integer %q: %w", s, err)
	}
	return val, nil
}
