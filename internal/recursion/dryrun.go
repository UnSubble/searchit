package recursion

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/unsubble/searchit/internal/wordlist"
)

// ScanDryRunRequest is a single URL candidate produced by the dry-run pipeline.
// It is generated using the same DirectoryGenerator.Next() path used by the real
// Manager.Run, guaranteeing candidate #N parity for depth-1 candidates.
type ScanDryRunRequest struct {
	// Index is the 1-based position of this candidate across all seeds.
	Index int
	// URL is the fully constructed candidate URL.
	URL string
}

// ScanDryRunConfig holds the configuration values needed to print the header.
type ScanDryRunConfig struct {
	Target      string
	Workers     int
	IsRecursive bool
	Strategy    string // "BFS", "DFS", "Priority" or ""
	MaxDepth    int
	Wordlist    string // path or "embedded"
}

// GenerateScanDryRunRequests runs the candidate-generation pipeline for scan
// without performing any network I/O.
//
// It constructs a DirectoryGenerator for each seed URL — the exact same
// generator that Manager.handleResult uses when expanding a discovered
// directory — and drains it via Next(). This guarantees:
//
//	Dry-run candidate #N == Real scan candidate #N   (depth 1)
//
// Deeper recursion (depth > 1) requires HTTP responses to determine which
// directories were found and whether recurse-on matches. Those candidates
// cannot be determined statically; the summary clearly reports this.
//
// limit controls how many ScanDryRunRequests are collected for display.
// 0 or negative means collect all. The function always iterates to completion
// for accurate total counting (but caps at wordlist size * len(seeds) to
// avoid runaway allocation).
//
// Returns:
//   - requests: the first min(limit, total) rendered requests
//   - total:    total candidates iterated (not limited by limit)
//   - err:      non-nil only on context cancellation or wordlist load error
func GenerateScanDryRunRequests(
	ctx context.Context,
	seeds []string,
	reader wordlist.Reader,
	normalizePaths bool,
	collapseSlashes bool,
	extensions []string,
	limit int,
) (requests []ScanDryRunRequest, total int64, err error) {
	// Shared visited set across seeds — same deduplication as the real manager.
	visited := make(map[string]struct{})

	for _, seed := range seeds {
		if ctx.Err() != nil {
			err = ctx.Err()
			return
		}

		gen, genErr := NewDirectoryGenerator(
			ctx,
			reader,
			seed,
			nil,   // parentPath — root has no parent path segments
			1,     // depth=1, same as Manager.handleResult passes for the first level
			"",    // parentResContentType — no HTTP response available
			nil,   // prioritizedSegments — adaptive disabled (no response)
			nil,   // prioritizedPaths    — adaptive disabled (no response)
			false, // laravel
			false, // wp
			false, // express
			normalizePaths,
			collapseSlashes,
			extensions,
			visited,
			nil, // fingerprintCache — adaptive disabled
			nil, // statsCollector
			nil, // highPriorityCounter
			nil, // lowPriorityCounter
		)
		if genErr != nil {
			err = genErr
			return
		}

		for {
			if ctx.Err() != nil {
				err = ctx.Err()
				return
			}
			job, ok := gen.Next()
			if !ok {
				break
			}
			total++
			if limit <= 0 || int(total) <= limit {
				requests = append(requests, ScanDryRunRequest{
					Index: int(total),
					URL:   job.URL,
				})
			}
		}
	}
	return requests, total, nil
}

// PrintScanDryRunHeader writes the DRY RUN banner and SCAN CONFIGURATION block to w.
func PrintScanDryRunHeader(w io.Writer, cfg ScanDryRunConfig) {
	fmt.Fprintln(w, "DRY RUN")
	fmt.Fprintln(w, strings.Repeat("─", 50))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SCAN CONFIGURATION")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Target\n    %s\n", cfg.Target)
	fmt.Fprintf(w, "  Workers\n    %d\n", cfg.Workers)
	if cfg.IsRecursive {
		fmt.Fprintln(w, "  Mode\n    Recursive")
		fmt.Fprintf(w, "  Strategy\n    %s\n", cfg.Strategy)
		fmt.Fprintf(w, "  Max Depth\n    %d\n", cfg.MaxDepth)
	} else {
		fmt.Fprintln(w, "  Mode\n    Standard")
	}
	fmt.Fprintf(w, "  Wordlist\n    %s\n", cfg.Wordlist)
}

// PrintScanDryRunSummary writes the DRY RUN SUMMARY block to w.
func PrintScanDryRunSummary(w io.Writer, previewed int, totalCandidates int64, isRecursive bool) {
	fmt.Fprintln(w, "DRY RUN SUMMARY")
	fmt.Fprintln(w)
	if totalCandidates > 0 {
		fmt.Fprintf(w, "  Candidates (depth 1)\n    %d\n", totalCandidates)
	} else {
		fmt.Fprintln(w, "  Candidates (depth 1)\n    unknown")
	}
	fmt.Fprintf(w, "  Previewed\n    %d\n", previewed)
	fmt.Fprintln(w, "  Requests Sent\n    0")
	fmt.Fprintln(w, "  Network Requests\n    0")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  No network requests were sent.")
	if isRecursive {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Note: Deeper recursive paths require HTTP responses and cannot be")
		fmt.Fprintln(w, "  discovered during dry-run.")
	}
}
