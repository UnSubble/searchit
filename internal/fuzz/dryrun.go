package fuzz

import (
	"context"
	"fmt"
	"strings"

	"github.com/unsubble/searchit/internal/engine"
)

// DryRunRequest is a fully rendered request produced by the dry-run pipeline.
// It is constructed using the identical IterateCandidates + BuildJob path as the
// live execution path, guaranteeing that dry-run candidate #N == real candidate #N.
type DryRunRequest struct {
	// Index is the 1-based position of this candidate in the iteration order.
	Index int
	// Req is the fully rendered RequestDTO that would be sent by the real executor.
	Req RequestDTO
}

// GenerateDryRunRequests runs the complete candidate-generation pipeline without
// performing any network I/O.
//
// It uses Runner.IterateCandidates and Runner.BuildJob — the exact same functions
// called by runEager — so candidate #N produced here is identical to candidate #N
// that would be sent by the real executor for the same configuration.
//
// limit controls how many rendered DryRunRequests are collected. Passing 0 or a
// negative value collects all candidates. The function always iterates to completion
// so that the returned total is accurate; it simply stops appending to results once
// limit is reached.
//
// Returns:
//   - results: the first min(limit, total) rendered requests
//   - total:   the total number of candidates iterated (not limited by limit)
//   - err:     non-nil only if the context was cancelled
func GenerateDryRunRequests(
	ctx context.Context,
	runner *Runner,
	primaryChan <-chan string,
	limit int,
) (results []DryRunRequest, total int64, err error) {
	// Compile templates exactly as Run() does, but without starting workers.
	runner.PrepareTemplates()
	urlTemplate := runner.CompiledURLTemplate()

	runner.IterateCandidates(ctx, primaryChan, func(vars map[string]string) bool {
		if ctx.Err() != nil {
			return false
		}
		total++
		if limit <= 0 || int(total) <= limit {
			job, buildErr := runner.BuildJob(urlTemplate, vars)
			if buildErr == nil {
				results = append(results, DryRunRequest{
					Index: int(total),
					Req:   job,
				})
			}
		}
		return true
	})

	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return results, total, err
}

// dryRunWriter is a minimal writer interface satisfied by io.Writer and also by
// the anonymous writer returned by cmd.OutOrStdout().
type dryRunWriter interface {
	Write([]byte) (int, error)
}

// PrintDryRunHeader writes the DRY RUN banner to w.
func PrintDryRunHeader(w dryRunWriter) {
	fmt.Fprintln(w, "DRY RUN")
	fmt.Fprintln(w, strings.Repeat("─", 50))
}

// PrintDryRunRequest writes a single numbered rendered request to w using the
// same indented section format as the live fuzz text output.
func PrintDryRunRequest(w dryRunWriter, dr DryRunRequest) {
	fmt.Fprintf(w, "REQUEST %d\n", dr.Index)
	fmt.Fprintf(w, "  URL\n    %s\n", dr.Req.URL)

	if dr.Req.FuzzData != nil {
		var (
			headers []engine.FuzzField
			cookies []engine.FuzzField
			bodies  []engine.FuzzField
		)
		for _, f := range dr.Req.FuzzData.Fields {
			switch f.Location {
			case engine.LocationHeader:
				headers = append(headers, f)
			case engine.LocationCookie:
				cookies = append(cookies, f)
			case engine.LocationBody, engine.LocationJSON:
				bodies = append(bodies, f)
			}
		}
		if len(headers) > 0 {
			fmt.Fprintln(w, "  Header")
			for _, h := range headers {
				if h.Name != "" {
					fmt.Fprintf(w, "    %s: %s\n", h.Name, h.Value)
				} else {
					fmt.Fprintf(w, "    %s\n", h.Value)
				}
			}
		}
		if len(cookies) > 0 {
			fmt.Fprintln(w, "  Cookie")
			for _, c := range cookies {
				if c.Name != "" {
					fmt.Fprintf(w, "    %s=%s\n", c.Name, c.Value)
				} else {
					fmt.Fprintf(w, "    %s\n", c.Value)
				}
			}
		}
		if len(bodies) > 0 {
			fmt.Fprintln(w, "  Body")
			for _, b := range bodies {
				for _, line := range strings.Split(b.Value, "\n") {
					fmt.Fprintf(w, "    %s\n", line)
				}
			}
		}
	}
	fmt.Fprintln(w)
}

// PrintDryRunSummary writes the DRY RUN SUMMARY block to w.
func PrintDryRunSummary(w dryRunWriter, totalCandidates int64, previewed int) {
	fmt.Fprintln(w, "DRY RUN SUMMARY")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Candidates       %d\n", totalCandidates)
	fmt.Fprintf(w, "  Previewed        %d\n", previewed)
	fmt.Fprintln(w, "  Requests Sent    0")
	fmt.Fprintln(w, "  Network Requests 0")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  No network requests were sent.")
}
