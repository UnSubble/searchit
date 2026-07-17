package config

import (
	"time"

	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
)

// TechProfile describes a technology-specific scanning profile selected via --tech.
type TechProfile struct {
	// ID is the canonical lowercase identifier (e.g. "laravel", "wordpress").
	ID string
	// DisplayName is the human-readable name shown in output (e.g. "Laravel").
	DisplayName string
}

// Config is the single source of truth consumed by the engine.
// All external inputs (CLI, YAML, env) must be translated into this struct.
type Config struct {
	URLs     []string
	Wordlist string

	Threads        int
	Timeout        time.Duration
	Delay          time.Duration
	Rate           float64
	ConnectTimeout time.Duration

	Recursive       bool
	MaxDepth        uint16
	Strategy        recursion.Strategy
	FollowRedirects bool

	RecurseOn status.Filters

	Paths PathConfig

	OutputFile   string // "" means write to stdout
	OutputFormat string // "text", "json", "ndjson", "csv", "markdown"
	Quiet        bool

	IncludeSize size.Filters
	ExcludeSize size.Filters

	IncludeHeaders []HeaderFilter
	ExcludeHeaders []HeaderFilter

	Status StatusConfig

	// Request Manipulation fields
	Method  string
	Data    string
	Headers []string
	Cookies string
	Proxy   string

	// Response Filtering fields
	MatchRegex    []string
	FilterRegex   []string
	MatchContent  []string
	FilterContent []string

	// TechProfile is the explicitly-selected technology profile (--tech flag).
	// A nil value means no explicit selection; automatic detection applies.
	TechProfile *TechProfile

	// Adaptive enables technology detection and adaptive path injection.
	Adaptive bool

	// Response Presentation fields
	ShowHeaders bool
	ShowTitle   bool

	// HTTP Request Templates
	RequestFile string
}

type HeaderFilter struct {
	Name  string
	Value string
}

type PathConfig struct {
	NormalizePaths  bool
	CollapseSlashes bool
}

type StatusConfig struct {
	Include status.Filters
	Exclude status.Filters
}
