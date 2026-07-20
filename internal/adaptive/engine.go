package adaptive

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/unsubble/searchit/internal/adaptive/prioritizer"
	"github.com/unsubble/searchit/internal/adaptive/selector"
	"github.com/unsubble/searchit/internal/adaptive/signals"
	"github.com/unsubble/searchit/internal/adaptive/summary"
	"github.com/unsubble/searchit/internal/fingerprint"
)

// Engine is the central target awareness layer shared across searchit components.
type Engine struct {
	TargetURL string
	Client    *http.Client
	Cache     *fingerprint.Cache
	Quiet     bool
	Collector *Collector
	Summary   *summary.Summary
}

// NewEngine instantiates a modular Adaptive Engine.
func NewEngine(targetURL string, client *http.Client, cache *fingerprint.Cache, quiet bool) *Engine {
	return &Engine{
		TargetURL: targetURL,
		Client:    client,
		Cache:     cache,
		Quiet:     quiet,
		Collector: NewCollector(targetURL, client, cache),
		Summary:   summary.NewSummary(),
	}
}

// Discover executes target signal collection.
func (e *Engine) Discover(ctx context.Context) error {
	if !e.Quiet {
		fmt.Fprintln(os.Stdout, "[INFO] Adaptive mode enabled.")
		fmt.Fprintln(os.Stdout, "[INFO] Discovering target...")
	}

	if err := e.Collector.Execute(ctx); err != nil {
		return err
	}

	// Populate summary technologies
	if e.Collector.LaravelDetected {
		e.Summary.Technologies = append(e.Summary.Technologies, "Laravel")
	}
	if e.Collector.WPDetected {
		e.Summary.Technologies = append(e.Summary.Technologies, "WordPress")
	}
	if e.Collector.ExpressDetected {
		e.Summary.Technologies = append(e.Summary.Technologies, "Express")
	}

	// Populate summary discoveries
	if e.Collector.RobotsDiscovered {
		e.Summary.Discoveries = append(e.Summary.Discoveries, "robots.txt")
	}
	if e.Collector.SitemapDiscovered {
		e.Summary.Discoveries = append(e.Summary.Discoveries, "sitemap.xml")
	}

	// Print discovery logging if !Quiet
	if !e.Quiet {
		if e.Collector.LaravelDetected {
			fmt.Fprintln(os.Stdout, "[INFO] Laravel detected")
		}
		if e.Collector.WPDetected {
			fmt.Fprintln(os.Stdout, "[INFO] WordPress detected")
		}
		if e.Collector.ExpressDetected {
			fmt.Fprintln(os.Stdout, "[INFO] Express detected")
		}
		if e.Collector.RobotsDiscovered {
			fmt.Fprintln(os.Stdout, "[INFO] robots.txt discovered")
		}
		if e.Collector.SitemapDiscovered {
			fmt.Fprintln(os.Stdout, "[INFO] sitemap.xml discovered")
		}
	}

	return nil
}

// GetSignals returns the list of matched signals for a given word.
func (e *Engine) GetSignals(word string, parentPath []string, depth int, parentResContentType string) []signals.SignalType {
	return prioritizer.CalculateSignals(
		word,
		parentPath,
		depth,
		parentResContentType,
		e.Collector.PrioritizedSegments,
		e.Collector.PrioritizedPaths,
		e.Collector.LaravelDetected,
		e.Collector.WPDetected,
		e.Collector.ExpressDetected,
	)
}

// GetScore calculates the priority score for a given word.
func (e *Engine) GetScore(word string, parentPath []string, depth int, parentResContentType string) int {
	sigs := e.GetSignals(word, parentPath, depth, parentResContentType)
	score := prioritizer.GetScore(sigs)
	e.Summary.RecordPriority(score)
	return score
}

// SelectTraversal evaluates the default rule table against the provided signals
// and returns a Decision containing the traversal Policy and the matched rule name.
func (e *Engine) SelectTraversal(sigs []signals.SignalType) selector.Decision {
	return selector.Select(selector.DefaultRules, sigs)
}
