// Package useragent provides User-Agent management for Searchit.
//
// It exposes an embedded list of modern desktop User-Agent strings, a
// function to select one at random, and [Resolve] to apply the precedence
// rules shared by both the scan and fuzz commands.
package useragent

import "math/rand"

// agents is the built-in list of modern desktop User-Agent strings.
// It is embedded in the binary; no external file or network access is needed.
var agents = []string{
	// Chrome – Windows
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	// Chrome – Linux
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	// Chrome – macOS
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	// Firefox – Windows
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:126.0) Gecko/20100101 Firefox/126.0",
	// Firefox – Linux
	"Mozilla/5.0 (X11; Linux x86_64; rv:126.0) Gecko/20100101 Firefox/126.0",
	// Firefox – macOS
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14.5; rv:126.0) Gecko/20100101 Firefox/126.0",
	// Firefox – Ubuntu
	"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:126.0) Gecko/20100101 Firefox/126.0",
	// Safari – macOS
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
	// Edge – Windows
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36 Edg/125.0.0.0",
}

// Agents returns the full list of built-in User-Agent strings.
// Callers that need to verify membership should use this slice.
func Agents() []string {
	cp := make([]string, len(agents))
	copy(cp, agents)
	return cp
}

// Random returns a randomly selected User-Agent from the built-in list.
// Callers should call this exactly once at startup and reuse the returned value
// for the lifetime of the command execution.
func Random() string {
	return agents[rand.Intn(len(agents))]
}

// Resolve returns the final User-Agent string following this precedence:
//
//  1. existingUA  — already present in headers via -H "User-Agent=..." (highest priority)
//  2. explicit    — from the --user-agent flag
//  3. randomUA    — pre-selected value supplied by the caller when --random-agent or
//     profile random-agent: true is in effect (both are treated identically)
//  4. ""          — no User-Agent is set; the HTTP client uses its own default
//
// randomUA must be the result of a single [Random] call made at startup, not
// a value produced inside a worker goroutine.
func Resolve(existingUA, explicit, randomUA string) string {
	if existingUA != "" {
		return existingUA
	}
	if explicit != "" {
		return explicit
	}
	if randomUA != "" {
		return randomUA
	}
	return ""
}
