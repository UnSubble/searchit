package prioritizer

import (
	"strings"

	"github.com/unsubble/searchit/internal/adaptive/signals"
)

// CalculateSignals checks which signals match a given candidate word.
func CalculateSignals(
	word string,
	parentPath []string,
	depth int,
	parentResContentType string,
	prioritizedSegments map[string]bool,
	prioritizedPaths map[string]bool,
	laravel, wp, express bool,
) []signals.SignalType {
	var sigs []signals.SignalType
	wordLower := strings.ToLower(word)

	// 1. Robots and sitemaps matching
	if prioritizedSegments[wordLower] {
		sigs = append(sigs, signals.SignalRobots)
	}
	fullCandidatePath := strings.Join(append(parentPath, wordLower), "/")
	if prioritizedPaths[fullCandidatePath] {
		sigs = append(sigs, signals.SignalSitemap)
	}

	// 2. Tech detection matching
	if laravel {
		sigs = append(sigs, signals.SignalLaravel)
		laravelWords := map[string]bool{
			"storage": true, "vendor": true, "horizon": true, "telescope": true, "artisan": true,
		}
		if laravelWords[wordLower] {
			sigs = append(sigs, signals.SignalAdmin)
		}
	}
	if wp {
		sigs = append(sigs, signals.SignalWordPress)
		wpWords := map[string]bool{
			"wp-admin": true, "wp-content": true, "wp-includes": true,
		}
		if wpWords[wordLower] {
			sigs = append(sigs, signals.SignalAdmin)
		}
	}
	if express {
		sigs = append(sigs, signals.SignalExpress)
		expressWords := map[string]bool{
			"api": true, "uploads": true, "assets": true, "static": true,
		}
		if expressWords[wordLower] {
			sigs = append(sigs, signals.SignalAdmin)
		}
	}

	// 3. Content-Type matching
	ctLower := strings.ToLower(parentResContentType)
	if strings.Contains(ctLower, "json") {
		sigs = append(sigs, signals.SignalJSON)
		apiWords := map[string]bool{
			"api": true, "v1": true, "rest": true, "swagger": true, "openapi": true,
		}
		if apiWords[wordLower] {
			sigs = append(sigs, signals.SignalAPI)
		}
		if wordLower == "graphql" {
			sigs = append(sigs, signals.SignalGraphQL)
		}
	}

	// 4. Low priority matching (asset)
	lowPriorityFolders := map[string]bool{
		"assets": true, "images": true, "css": true, "js": true, "static": true,
		"temp": true, "tmp": true, "vendor": true, "node_modules": true,
	}
	if lowPriorityFolders[wordLower] {
		sigs = append(sigs, signals.SignalAsset)
	}

	// 5. Explicit admin word check
	if wordLower == "admin" {
		sigs = append(sigs, signals.SignalAdmin)
	}

	return sigs
}

// GetScore calculates the sum of scores for all matched signals.
func GetScore(sigs []signals.SignalType) int {
	score := 0
	for _, sig := range sigs {
		score += signals.ScoreMap[sig]
	}
	return score
}
