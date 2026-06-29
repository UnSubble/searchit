package http

import (
	"strconv"
)

type StatusCode string

func (s StatusCode) Match(code int) bool {
	pattern := string(s)

	if len(pattern) != 3 {
		return false
	}

	actual := strconv.Itoa(code)

	for i := 0; i < 3; i++ {
		if pattern[i] == 'x' || pattern[i] == 'X' {
			continue
		}

		if pattern[i] != actual[i] {
			return false
		}
	}

	return true
}
