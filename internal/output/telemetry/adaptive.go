package telemetry

import (
	"io"
	"strconv"
	"strings"

	"github.com/unsubble/searchit/internal/output/terminal"
)

type AdaptiveInfo struct {
	Technologies        []string
	Discoveries         []string
	DFSCount            int
	BFSCount            int
	EagerCount          int
	HighPriorityCount   int
	MediumPriorityCount int
	LowPriorityCount    int
}

// PrintAdaptive prints the adaptive strategy summary block.
// All output is routed through tm.Emit(owner, fn).
func PrintAdaptive(tm *terminal.Manager, owner terminal.Owner, info AdaptiveInfo) {
	techStr := "None detected"
	if len(info.Technologies) > 0 {
		techStr = strings.Join(info.Technologies, ", ")
	}
	discStr := "None"
	if len(info.Discoveries) > 0 {
		discStr = strings.Join(info.Discoveries, ", ")
	}

	items := []terminal.Item{
		{Key: "Technologies", Value: techStr},
		{Key: "Discoveries", Value: discStr},
		{Key: "DFS Policies", Value: strconv.Itoa(info.DFSCount)},
		{Key: "BFS Policies", Value: strconv.Itoa(info.BFSCount)},
		{Key: "Eager Policies", Value: strconv.Itoa(info.EagerCount)},
		{Key: "High Priority", Value: strconv.Itoa(info.HighPriorityCount)},
		{Key: "Medium Priority", Value: strconv.Itoa(info.MediumPriorityCount)},
		{Key: "Low Priority", Value: strconv.Itoa(info.LowPriorityCount)},
	}

	_ = tm.Emit(owner, func(w io.Writer) {
		terminal.RenderBlock(w, "ADAPTIVE SUMMARY", items, tm.ContentWidth())
	})
}
