package telemetry

import (
	"io"
	"strconv"
	"strings"

	"github.com/unsubble/searchit/internal/output"
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

func PrintNormalConfigurationWithWidth(w io.Writer, info ConfigInfo, width int) {
	adaptiveStr := "disabled"
	if info.AdaptiveEnabled {
		adaptiveStr = "enabled"
	}

	wl := info.PrimaryWordlist
	if wl == "" {
		wl = "embedded"
	}

	items := []output.Item{
		{Key: "Target", Value: info.Target},
		{Key: "Strategy", Value: info.Strategy},
		{Key: "Adaptive", Value: adaptiveStr},
		{Key: "Wordlist", Value: wl},
	}

	if len(info.Extensions) > 0 {
		items = append(items, output.Item{Key: "Extensions", Value: strings.Join(info.Extensions, ", ")})
	}
	items = append(items, output.Item{Key: "Workers", Value: strconv.Itoa(info.Workers)})

	title := "SCAN CONFIGURATION"
	if info.IsFuzz {
		title = "FUZZ CONFIGURATION"
	}

	output.RenderBlock(w, title, items, width)
}

func PrintNormalConfiguration(w io.Writer, info ConfigInfo) {
	PrintNormalConfigurationWithWidth(w, info, 0)
}

func PrintConfigurationWithWidth(w io.Writer, info ConfigInfo, width int) {
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

	items := []output.Item{
		{Key: "Target", Value: info.Target},
		{Key: "Method", Value: info.Method},
		{Key: "Workers", Value: strconv.Itoa(info.Workers)},
		{Key: "Strategy", Value: info.Strategy},
		{Key: "Adaptive", Value: adaptiveStr},
	}

	if info.IsFuzz {
		items = append(items,
			output.Item{Key: "Wordlists", Value: strconv.Itoa(info.WordlistsCount)},
			output.Item{Key: "Primary Wordlist", Value: wl},
			output.Item{Key: "Placeholders", Value: info.Placeholders},
		)
	} else {
		items = append(items, output.Item{Key: "Wordlist", Value: wl})
	}

	if len(info.Extensions) > 0 {
		items = append(items, output.Item{Key: "Extensions", Value: strings.Join(info.Extensions, ", ")})
	}

	httpVer := info.HTTPVersion
	if httpVer == "" {
		httpVer = "HTTP/1.1"
	}

	items = append(items,
		output.Item{Key: "HTTP Version", Value: httpVer},
		output.Item{Key: "Follow Redirects", Value: redirStr},
		output.Item{Key: "Filter Status", Value: info.FilterStatus},
		output.Item{Key: "Total Candidates", Value: candStr},
	)

	title := "SCAN CONFIGURATION"
	if info.IsFuzz {
		title = "FUZZ CONFIGURATION"
	}

	output.RenderBlock(w, title, items, width)
}

func PrintConfiguration(w io.Writer, info ConfigInfo) {
	PrintConfigurationWithWidth(w, info, 0)
}
