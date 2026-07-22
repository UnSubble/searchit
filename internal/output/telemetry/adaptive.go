package telemetry

import (
	"io"
	"strconv"
	"strings"

	"github.com/unsubble/searchit/internal/output"
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

func PrintAdaptiveWithWidth(w io.Writer, info AdaptiveInfo, width int) {
	techStr := "None detected"
	if len(info.Technologies) > 0 {
		techStr = strings.Join(info.Technologies, ", ")
	}
	discStr := "None"
	if len(info.Discoveries) > 0 {
		discStr = strings.Join(info.Discoveries, ", ")
	}

	items := []output.Item{
		{Key: "Technologies", Value: techStr},
		{Key: "Discoveries", Value: discStr},
		{Key: "DFS Policies", Value: strconv.Itoa(info.DFSCount)},
		{Key: "BFS Policies", Value: strconv.Itoa(info.BFSCount)},
		{Key: "Eager Policies", Value: strconv.Itoa(info.EagerCount)},
		{Key: "High Priority", Value: strconv.Itoa(info.HighPriorityCount)},
		{Key: "Medium Priority", Value: strconv.Itoa(info.MediumPriorityCount)},
		{Key: "Low Priority", Value: strconv.Itoa(info.LowPriorityCount)},
	}

	output.RenderBlock(w, "ADAPTIVE SUMMARY", items, width)
}

func PrintAdaptive(w io.Writer, info AdaptiveInfo) {
	PrintAdaptiveWithWidth(w, info, 0)
}
