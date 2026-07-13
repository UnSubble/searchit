package robots

import (
	"bufio"
	"io"
	"strings"
)

// Type represents the kind of robots.txt directive.
type Type int

const (
	Allow Type = iota
	Disallow
	Sitemap
)

// Directive is a single robots.txt path rule.
type Directive struct {
	Type Type
	Path string
}

// Parse extracts Allow, Disallow, and Sitemap paths from a robots.txt stream.
// It parses line-by-line using bufio.Scanner to keep allocations and
// memory footprint minimal. It ignores comments and unsupported directives.
func Parse(r io.Reader) ([]Directive, error) {
	var directives []Directive
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		// 1. Remove comments
		if idx := strings.Index(line, "#"); idx != -1 {
			line = line[:idx]
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 2. Extract key-value pair
		idx := strings.Index(line, ":")
		if idx == -1 {
			continue // ignore malformed lines
		}

		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		val := strings.TrimSpace(line[idx+1:])

		var dirType Type
		switch key {
		case "allow":
			dirType = Allow
		case "disallow":
			dirType = Disallow
		case "sitemap":
			dirType = Sitemap
		default:
			continue // ignore unsupported directives (e.g. User-agent, Crawl-delay)
		}

		directives = append(directives, Directive{
			Type: dirType,
			Path: val,
		})
	}

	return directives, scanner.Err()
}
