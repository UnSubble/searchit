package summary

import (
	"fmt"
	"io"
	"sync"

	"github.com/unsubble/searchit/internal/adaptive/types"
)

// Summary accumulates execution metrics to print a final report.
type Summary struct {
	mu           sync.Mutex
	Technologies []string
	Discoveries  []string
	Candidates   int
	DFSCount     int
	BFSCount     int
	EagerCount   int
	Findings     int

	HighPriorityCount   int
	MediumPriorityCount int
	LowPriorityCount    int
}

// NewSummary returns an initialized empty Summary.
func NewSummary() *Summary {
	return &Summary{}
}

// RecordPriority increments candidate priority based on score.
func (s *Summary) RecordPriority(score int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if score >= 30 {
		s.HighPriorityCount++
	} else if score > 0 {
		s.MediumPriorityCount++
	} else {
		s.LowPriorityCount++
	}
}

// RecordTraversal increments traversal category counters and candidates count.
func (s *Summary) RecordTraversal(p types.Policy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Candidates++
	switch p {
	case types.PolicyDFS:
		s.DFSCount++
	case types.PolicyEager:
		s.EagerCount++
	default:
		s.BFSCount++
	}
}

// RecordFindings sets findings count.
func (s *Summary) RecordFindings(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Findings = n
}

// Print formats and writes the summary to the provided writer.
func (s *Summary) Print(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Fprintln(w, "------------------------")
	fmt.Fprintln(w, "ADAPTIVE SUMMARY")
	fmt.Fprintln(w, "------------------------")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Technologies:")
	if len(s.Technologies) == 0 {
		fmt.Fprintln(w, "    None detected")
	} else {
		for _, tech := range s.Technologies {
			fmt.Fprintf(w, "    %s\n", tech)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Discoveries:")
	if len(s.Discoveries) == 0 {
		fmt.Fprintln(w, "    None")
	} else {
		for _, disc := range s.Discoveries {
			fmt.Fprintf(w, "    %s\n", disc)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Candidates:\n")
	fmt.Fprintf(w, "    %d\n\n", s.Candidates)

	fmt.Fprintf(w, "Policies:\n")
	fmt.Fprintf(w, "    DFS .......... %d\n", s.DFSCount)
	fmt.Fprintf(w, "    BFS ......... %d\n", s.BFSCount)
	fmt.Fprintf(w, "    EAGER ........ %d\n\n", s.EagerCount)

	fmt.Fprintf(w, "Findings:\n")
	fmt.Fprintf(w, "    %d\n\n", s.Findings)

	fmt.Fprintf(w, "Adaptive:\n")
	fmt.Fprintln(w, "    Target aware prioritization")
	fmt.Fprintln(w, "    Deterministic scheduling")
	fmt.Fprintln(w, "    Signal based traversal")
}
