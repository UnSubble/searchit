package telemetry

import (
	"io"
	"strconv"
	"strings"

	"github.com/unsubble/searchit/internal/output/terminal"
	"github.com/unsubble/searchit/internal/presentation"
)

type ConfigInfo struct {
	Target          string
	Method          string
	Workers         int
	Mode            string
	Traversal       string
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
		{Key: "Target", Value: presentation.URL(info.Target, 45)},
		{Key: "Mode", Value: info.Mode},
	}
	if info.Traversal != "" {
		items = append(items, terminal.Item{Key: "Traversal", Value: info.Traversal})
	}
	items = append(items,
		terminal.Item{Key: "Adaptive", Value: adaptiveStr},
		terminal.Item{Key: "Wordlist", Value: presentation.Path(wl, 35)},
	)

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
	wl := info.PrimaryWordlist
	if wl == "" {
		wl = "embedded"
	} else {
		wl = presentation.Path(wl, 35)
	}

	items := []terminal.Item{
		{Key: "Target", Value: presentation.URL(info.Target, 45)},
	}

	if info.Method != "" {
		items = append(items, terminal.Item{Key: "Method", Value: info.Method})
	}

	items = append(items, terminal.Item{Key: "Workers", Value: strconv.Itoa(info.Workers)})
	items = append(items, terminal.Item{Key: "Mode", Value: info.Mode})

	if info.Traversal != "" {
		items = append(items, terminal.Item{Key: "Traversal", Value: info.Traversal})
	}
	if info.AdaptiveEnabled {
		items = append(items, terminal.Item{Key: "Adaptive", Value: "enabled"})
	}

	if info.IsFuzz {
		items = append(items,
			terminal.Item{Key: "Wordlists", Value: strconv.Itoa(info.WordlistsCount)},
			terminal.Item{Key: "Primary Wordlist", Value: wl},
		)
		if info.Placeholders != "" {
			items = append(items, terminal.Item{Key: "Placeholders", Value: info.Placeholders})
		}
	} else {
		items = append(items, terminal.Item{Key: "Wordlist", Value: wl})
	}

	if len(info.Extensions) > 0 {
		items = append(items, terminal.Item{Key: "Extensions", Value: strings.Join(info.Extensions, ", ")})
	}

	if info.HTTPVersion != "" && info.HTTPVersion != "HTTP/1.1" {
		items = append(items, terminal.Item{Key: "HTTP Version", Value: info.HTTPVersion})
	}

	if info.FollowRedirects {
		items = append(items, terminal.Item{Key: "Follow Redirects", Value: "true"})
	}

	if info.FilterStatus != "" {
		items = append(items, terminal.Item{Key: "Filter Status", Value: info.FilterStatus})
	}

	title := "SCAN CONFIGURATION"
	if info.IsFuzz {
		title = "FUZZ CONFIGURATION"
	}

	_ = tm.Emit(owner, func(w io.Writer) {
		terminal.RenderBlock(w, title, items, tm.ContentWidth())
	})
}
