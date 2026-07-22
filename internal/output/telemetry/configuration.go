package telemetry

import (
	"io"
	"strconv"
	"strings"

	"github.com/unsubble/searchit/internal/output/terminal"
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
	Extensions      []string
}

// PrintNormalConfiguration prints a compact configuration block.
// All output is routed through tm.Emit(owner, fn).
func PrintNormalConfiguration(tm *terminal.Manager, owner terminal.Owner, info ConfigInfo) {
	adaptiveStr := "disabled"
	if info.AdaptiveEnabled {
		adaptiveStr = "enabled"
	}

	wl := info.PrimaryWordlist
	if wl == "" {
		wl = "embedded"
	}

	items := []terminal.Item{
		{Key: "Target", Value: info.Target},
		{Key: "Strategy", Value: info.Strategy},
		{Key: "Adaptive", Value: adaptiveStr},
		{Key: "Wordlist", Value: wl},
	}

	if len(info.Extensions) > 0 {
		items = append(items, terminal.Item{Key: "Extensions", Value: strings.Join(info.Extensions, ", ")})
	}
	items = append(items, terminal.Item{Key: "Workers", Value: strconv.Itoa(info.Workers)})

	title := "SCAN CONFIGURATION"
	if info.IsFuzz {
		title = "FUZZ CONFIGURATION"
	}

	_ = tm.Emit(owner, func(w io.Writer) {
		terminal.RenderBlock(w, title, items, tm.ContentWidth())
	})
}

// PrintConfiguration prints the full configuration block including HTTP details.
// All output is routed through tm.Emit(owner, fn).
func PrintConfiguration(tm *terminal.Manager, owner terminal.Owner, info ConfigInfo) {
	adaptiveStr := "disabled"
	if info.AdaptiveEnabled {
		adaptiveStr = "enabled"
	}

	wl := info.PrimaryWordlist
	if wl == "" {
		wl = "embedded"
	}

	redirStr := "false"
	if info.FollowRedirects {
		redirStr = "true"
	}

	candStr := "unknown"
	if info.TotalCandidates >= 0 {
		candStr = strconv.Itoa(info.TotalCandidates)
	}

	items := []terminal.Item{
		{Key: "Target", Value: info.Target},
		{Key: "Method", Value: info.Method},
		{Key: "Workers", Value: strconv.Itoa(info.Workers)},
		{Key: "Strategy", Value: info.Strategy},
		{Key: "Adaptive", Value: adaptiveStr},
	}

	if info.IsFuzz {
		items = append(items,
			terminal.Item{Key: "Wordlists", Value: strconv.Itoa(info.WordlistsCount)},
			terminal.Item{Key: "Primary Wordlist", Value: wl},
			terminal.Item{Key: "Placeholders", Value: info.Placeholders},
		)
	} else {
		items = append(items, terminal.Item{Key: "Wordlist", Value: wl})
	}

	if len(info.Extensions) > 0 {
		items = append(items, terminal.Item{Key: "Extensions", Value: strings.Join(info.Extensions, ", ")})
	}

	httpVer := info.HTTPVersion
	if httpVer == "" {
		httpVer = "HTTP/1.1"
	}

	items = append(items,
		terminal.Item{Key: "HTTP Version", Value: httpVer},
		terminal.Item{Key: "Follow Redirects", Value: redirStr},
		terminal.Item{Key: "Filter Status", Value: info.FilterStatus},
		terminal.Item{Key: "Total Candidates", Value: candStr},
	)

	title := "SCAN CONFIGURATION"
	if info.IsFuzz {
		title = "FUZZ CONFIGURATION"
	}

	_ = tm.Emit(owner, func(w io.Writer) {
		terminal.RenderBlock(w, title, items, tm.ContentWidth())
	})
}
