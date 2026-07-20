package telemetry

import (
	"fmt"
	"io"
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

func PrintAdaptive(w io.Writer, info AdaptiveInfo) {
	fmt.Fprintln(w, "---------------------------------------------------------")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "ADAPTIVE SUMMARY")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Technologies:")
	if len(info.Technologies) == 0 {
		fmt.Fprintln(w, "    None detected")
	} else {
		for _, tech := range info.Technologies {
			fmt.Fprintf(w, "    %s\n", tech)
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Discoveries:")
	if len(info.Discoveries) == 0 {
		fmt.Fprintln(w, "    None")
	} else {
		for _, disc := range info.Discoveries {
			fmt.Fprintf(w, "    %s\n", disc)
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Policies:")
	fmt.Fprintf(w, "    %s%d\n", padDots("DFS"), info.DFSCount)
	fmt.Fprintf(w, "    %s%d\n", padDots("BFS"), info.BFSCount)
	fmt.Fprintf(w, "    %s%d\n\n", padDots("EAGER"), info.EagerCount)

	fmt.Fprintln(w, "Candidates:")
	fmt.Fprintf(w, "    %s%d\n", padDots("High priority"), info.HighPriorityCount)
	fmt.Fprintf(w, "    %s%d\n", padDots("Medium priority"), info.MediumPriorityCount)
	fmt.Fprintf(w, "    %s%d\n\n", padDots("Low priority"), info.LowPriorityCount)

	fmt.Fprintln(w, "Adaptive:")
	fmt.Fprintln(w, "    Target aware prioritization")
	fmt.Fprintln(w, "    Deterministic scheduling")
	fmt.Fprintln(w, "    Signal based traversal")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "---------------------------------------------------------")
}
