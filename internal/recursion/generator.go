package recursion

import (
	"context"
	"sync/atomic"

	"github.com/unsubble/searchit/internal/adaptive/prioritizer"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/extensions"
	"github.com/unsubble/searchit/internal/fingerprint"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/wordlist"
)

// Generator defines a lazy stream of jobs.
type Generator interface {
	// Next returns the next job. If ok is false, the generator is exhausted.
	Next() (engine.Job, bool)
}

// SliceGenerator yields a predefined slice of jobs.
type SliceGenerator struct {
	jobs []engine.Job
	idx  int
}

// NewSliceGenerator creates a generator from a slice of jobs.
func NewSliceGenerator(jobs []engine.Job) *SliceGenerator {
	return &SliceGenerator{jobs: jobs}
}

// Next yields the next job from the slice.
func (g *SliceGenerator) Next() (engine.Job, bool) {
	if g.idx >= len(g.jobs) {
		return engine.Job{}, false
	}
	job := g.jobs[g.idx]
	g.idx++
	return job, true
}

// DirectoryGenerator lazily streams a wordlist, applying filtering, extensions, and adaptive scoring.
type DirectoryGenerator struct {
	parentURL            string
	parentPath           []string
	depth                int
	parentResContentType string
	prioritizedSegments  map[string]bool
	prioritizedPaths     map[string]bool
	laravel, wp, express bool

	normalizePaths      bool
	collapseSlashes     bool
	extensions          []string
	visited             map[string]struct{}
	fingerprintCache    *fingerprint.Cache
	statsCollector      *stats.Collector
	highPriorityCounter *int
	lowPriorityCounter  *int

	words   chan string
	readErr chan error
	buffer  []engine.Job
}

// NewDirectoryGenerator constructs a generator and begins reading the wordlist asynchronously.
func NewDirectoryGenerator(
	ctx context.Context,
	reader wordlist.Reader,
	parentURL string,
	parentPath []string,
	depth int,
	parentResContentType string,
	prioritizedSegments map[string]bool,
	prioritizedPaths map[string]bool,
	laravel bool,
	wp bool,
	express bool,
	normalizePaths bool,
	collapseSlashes bool,
	extensions []string,
	visited map[string]struct{},
	fingerprintCache *fingerprint.Cache,
	statsCollector *stats.Collector,
	highPriorityCounter *int,
	lowPriorityCounter *int,
) *DirectoryGenerator {
	g := &DirectoryGenerator{
		parentURL:            parentURL,
		parentPath:           parentPath,
		depth:                depth,
		parentResContentType: parentResContentType,
		prioritizedSegments:  prioritizedSegments,
		prioritizedPaths:     prioritizedPaths,
		laravel:              laravel,
		wp:                   wp,
		express:              express,
		normalizePaths:       normalizePaths,
		collapseSlashes:      collapseSlashes,
		extensions:           extensions,
		visited:              visited,
		fingerprintCache:     fingerprintCache,
		statsCollector:       statsCollector,
		highPriorityCounter:  highPriorityCounter,
		lowPriorityCounter:   lowPriorityCounter,
		words:                make(chan string, wordlist.DefaultWordBuffer),
		readErr:              make(chan error, 1),
	}

	go func() {
		defer close(g.words)
		g.readErr <- reader.Read(ctx, g.words)
	}()

	return g
}

// Next yields the next generated job from the wordlist stream.
func (g *DirectoryGenerator) Next() (engine.Job, bool) {
	if len(g.buffer) > 0 {
		job := g.buffer[0]
		g.buffer = g.buffer[1:]
		return job, true
	}

	for word := range g.words {
		cleaned, ok := wordlist.CleanWord(word, g.normalizePaths, g.collapseSlashes)
		if !ok {
			continue
		}

		variants := extensions.GenerateVariants(cleaned, g.extensions)
		for _, variant := range variants {
			childURL, err := wordlist.Join(g.parentURL, variant)
			if err != nil {
				continue
			}
			key := normalizeURL(childURL)
			if _, seen := g.visited[key]; seen {
				continue
			}

			// Adaptive scoring
			score := 0
			isAdaptive := g.fingerprintCache != nil
			if isAdaptive {
				sigs := prioritizer.CalculateSignals(
					variant,
					g.parentPath,
					g.depth,
					g.parentResContentType,
					g.prioritizedSegments,
					g.prioritizedPaths,
					g.laravel,
					g.wp,
					g.express,
				)
				score = prioritizer.GetScore(sigs)
			}

			g.visited[key] = struct{}{}
			atomic.AddInt64(&stats.GlobalInstrumentation.JobsAccepted, 1)
			atomic.AddInt64(&stats.GlobalInstrumentation.JobsProduced, 1)

			if isAdaptive && score > 50 {
				if g.highPriorityCounter != nil {
					*g.highPriorityCounter++
				}
				g.buffer = append(g.buffer, engine.Job{URL: childURL, Depth: uint16(g.depth), Origin: engine.OriginWordlist})
			} else {
				if g.lowPriorityCounter != nil {
					*g.lowPriorityCounter++
				}
				g.buffer = append(g.buffer, engine.Job{URL: childURL, Depth: uint16(g.depth), Origin: engine.OriginWordlist})
			}
		}

		if len(g.buffer) > 0 {
			job := g.buffer[0]
			g.buffer = g.buffer[1:]
			return job, true
		}
	}

	<-g.readErr
	return engine.Job{}, false
}
