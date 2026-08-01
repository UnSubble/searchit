package adaptive

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/unsubble/searchit/internal/adaptive/prioritizer"
	"github.com/unsubble/searchit/internal/adaptive/selector"
	"github.com/unsubble/searchit/internal/adaptive/signals"
	"github.com/unsubble/searchit/internal/adaptive/summary"
	"github.com/unsubble/searchit/internal/engine"
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

	once    sync.Once
	discErr error
	mu      sync.Mutex
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

// Discover executes target signal collection idempotently with a bounded timeout.
func (e *Engine) Discover(ctx context.Context) error {
	e.once.Do(func() {
		if !e.Quiet {
			fmt.Fprintln(os.Stderr, "[INFO] Adaptive mode enabled.")
			fmt.Fprintln(os.Stderr, "[INFO] Discovering target...")
		}

		discCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if err := e.Collector.Execute(discCtx); err != nil {
			e.discErr = err
			return
		}

		e.mu.Lock()
		defer e.mu.Unlock()

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
				fmt.Fprintln(os.Stderr, "[INFO] Laravel detected")
			}
			if e.Collector.WPDetected {
				fmt.Fprintln(os.Stderr, "[INFO] WordPress detected")
			}
			if e.Collector.ExpressDetected {
				fmt.Fprintln(os.Stderr, "[INFO] Express detected")
			}
			if e.Collector.RobotsDiscovered {
				fmt.Fprintln(os.Stderr, "[INFO] robots.txt discovered")
			}
			if e.Collector.SitemapDiscovered {
				fmt.Fprintln(os.Stderr, "[INFO] sitemap.xml discovered")
			}
		}
	})

	return e.discErr
}

// GetDiscoveredJobs returns a defensive copy of all jobs discovered during target analysis.
func (e *Engine) GetDiscoveredJobs() []engine.Job {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.Collector.DiscoveredJobs) == 0 {
		return nil
	}

	copied := make([]engine.Job, len(e.Collector.DiscoveredJobs))
	copy(copied, e.Collector.DiscoveredJobs)
	return copied
}

// CheckRuntimeTech checks for runtime framework matches and returns any newly generated framework jobs.
func (e *Engine) CheckRuntimeTech(hostRoot string, host string) []engine.Job {
	e.mu.Lock()
	defer e.mu.Unlock()

	newJobs := e.Collector.DetectAndInjectFrameworks(hostRoot, host)

	// Update summary technologies if new framework detected
	if e.Collector.LaravelDetected && !contains(e.Summary.Technologies, "Laravel") {
		e.Summary.Technologies = append(e.Summary.Technologies, "Laravel")
	}
	if e.Collector.WPDetected && !contains(e.Summary.Technologies, "WordPress") {
		e.Summary.Technologies = append(e.Summary.Technologies, "WordPress")
	}
	if e.Collector.ExpressDetected && !contains(e.Summary.Technologies, "Express") {
		e.Summary.Technologies = append(e.Summary.Technologies, "Express")
	}

	return newJobs
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
	e.mu.Lock()
	defer e.mu.Unlock()

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

// GetMetrics returns adaptive telemetry metrics.
func (e *Engine) GetMetrics() (techs []string, discoveries []string, high, med, low int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	techsCopy := make([]string, len(e.Summary.Technologies))
	copy(techsCopy, e.Summary.Technologies)

	discCopy := make([]string, len(e.Summary.Discoveries))
	copy(discCopy, e.Summary.Discoveries)

	return techsCopy, discCopy, e.Summary.HighPriorityCount, e.Summary.MediumPriorityCount, e.Summary.LowPriorityCount
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
