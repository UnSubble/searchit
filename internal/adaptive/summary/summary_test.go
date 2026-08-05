package summary_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/unsubble/searchit/internal/adaptive/summary"
	"github.com/unsubble/searchit/internal/adaptive/types"
)

func TestSummary_RecordAndPrint(t *testing.T) {
	s := summary.NewSummary()
	s.Technologies = []string{"Laravel", "Vue.js"}
	s.Discoveries = []string{"/robots.txt", "/sitemap.xml"}

	s.RecordPriority(40) // High
	s.RecordPriority(20) // Medium
	s.RecordPriority(0)  // Low

	s.RecordTraversal(types.PolicyDFS)
	s.RecordTraversal(types.PolicyBFS)
	s.RecordTraversal(types.PolicyEager)

	s.RecordFindings(12)

	if s.HighPriorityCount != 1 || s.MediumPriorityCount != 1 || s.LowPriorityCount != 1 {
		t.Errorf("unexpected priority counts: high=%d, med=%d, low=%d",
			s.HighPriorityCount, s.MediumPriorityCount, s.LowPriorityCount)
	}

	if s.Candidates != 3 || s.DFSCount != 1 || s.BFSCount != 1 || s.EagerCount != 1 {
		t.Errorf("unexpected traversal counts: candidates=%d, dfs=%d, bfs=%d, eager=%d",
			s.Candidates, s.DFSCount, s.BFSCount, s.EagerCount)
	}

	if s.Findings != 12 {
		t.Errorf("expected findings = 12, got %d", s.Findings)
	}

	var buf bytes.Buffer
	s.Print(&buf)
	out := buf.String()

	if !strings.Contains(out, "ADAPTIVE SUMMARY") {
		t.Errorf("output missing header:\n%s", out)
	}
	if !strings.Contains(out, "Laravel") || !strings.Contains(out, "Vue.js") {
		t.Errorf("output missing technologies:\n%s", out)
	}
	if !strings.Contains(out, "/robots.txt") {
		t.Errorf("output missing discoveries:\n%s", out)
	}
}

func TestSummary_EmptyPrint(t *testing.T) {
	s := summary.NewSummary()
	var buf bytes.Buffer
	s.Print(&buf)
	out := buf.String()

	if !strings.Contains(out, "None detected") {
		t.Errorf("expected 'None detected' for technologies in empty summary, got:\n%s", out)
	}
	if !strings.Contains(out, "None") {
		t.Errorf("expected 'None' for discoveries in empty summary, got:\n%s", out)
	}
}

func TestSummary_ConcurrentSafety(t *testing.T) {
	s := summary.NewSummary()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s.RecordPriority(idx % 50)
			s.RecordTraversal(types.Policy(idx % 3))
		}(i)
	}
	wg.Wait()

	if s.Candidates != 50 {
		t.Errorf("expected 50 candidates, got %d", s.Candidates)
	}
}
