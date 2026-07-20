package telemetry

import (
	"fmt"
	"io"
	"strings"
)

type ConfigInfo struct {
	Target          string
	Method          string
	Workers         int
	Strategy        string
	AdaptiveEnabled bool
	WordlistsCount  int
	PrimaryWordlist string
	Placeholders    string // e.g. "FUZZ (1)"
	HTTPVersion     string // e.g. "auto"
	FollowRedirects bool
	FilterStatus    string // e.g. "40x"
	TotalCandidates int
	IsFuzz          bool
}

func padConfigDots(label string) string {
	targetWidth := 26
	if len(label) >= targetWidth {
		return label + " "
	}
	dotsCount := targetWidth - len(label) - 1
	if dotsCount < 0 {
		dotsCount = 0
	}
	return label + " " + strings.Repeat(".", dotsCount) + " "
}

func PrintNormalConfiguration(w io.Writer, info ConfigInfo) {
	adaptiveStr := "disabled"
	if info.AdaptiveEnabled {
		adaptiveStr = "enabled"
	}

	wl := info.PrimaryWordlist
	if wl == "" {
		wl = "embedded"
	}

	if info.IsFuzz {
		fmt.Fprintf(w, "%s%s\n", padConfigDots("[*] Fuzzing target"), info.Target)
	} else {
		fmt.Fprintf(w, "%s%s\n", padConfigDots("[*] Target"), info.Target)
	}
	fmt.Fprintf(w, "%s%s\n", padConfigDots("[*] Strategy"), info.Strategy)
	fmt.Fprintf(w, "%s%s\n", padConfigDots("[*] Adaptive"), adaptiveStr)
	fmt.Fprintf(w, "%s%s\n", padConfigDots("[*] Wordlist"), wl)
	fmt.Fprintf(w, "%s%d\n\n", padConfigDots("[*] Workers"), info.Workers)
}

func PrintConfiguration(w io.Writer, info ConfigInfo) {
	fmt.Fprintln(w, "---------------------------------------------------------")
	if info.IsFuzz {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "FUZZ CONFIGURATION")
	} else {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "SCAN CONFIGURATION")
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Target:\n    %s\n\n", info.Target)
	fmt.Fprintf(w, "Method:\n    %s\n\n", info.Method)
	fmt.Fprintf(w, "Workers:\n    %d\n\n", info.Workers)
	fmt.Fprintf(w, "Strategy:\n    %s\n\n", info.Strategy)

	adaptiveStr := "disabled"
	if info.AdaptiveEnabled {
		adaptiveStr = "enabled"
	}
	fmt.Fprintf(w, "Adaptive:\n    %s\n\n", adaptiveStr)

	if info.IsFuzz {
		fmt.Fprintf(w, "Wordlists:\n    %d\n\n", info.WordlistsCount)
		fmt.Fprintf(w, "Primary Wordlist:\n    %s\n\n", info.PrimaryWordlist)
		fmt.Fprintf(w, "Placeholders:\n    %s\n\n", info.Placeholders)
	} else {
		fmt.Fprintf(w, "Wordlist:\n    %s\n\n", info.PrimaryWordlist)
	}

	fmt.Fprintf(w, "HTTP Version:\n    %s\n\n", info.HTTPVersion)

	redirStr := "false"
	if info.FollowRedirects {
		redirStr = "true"
	}
	fmt.Fprintf(w, "Follow Redirects:\n    %s\n\n", redirStr)

	fmt.Fprintf(w, "Filter Status:\n    %s\n\n", info.FilterStatus)

	if info.TotalCandidates >= 0 {
		fmt.Fprintf(w, "Total Candidates:\n    %d\n", info.TotalCandidates)
	} else {
		fmt.Fprintf(w, "Total Candidates:\n    unknown\n")
	}
	fmt.Fprintln(w, "---------------------------------------------------------")
}
