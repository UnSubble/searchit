package config

import (
	"time"

	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
)

// Config is the single source of truth consumed by the engine.
// All external inputs (CLI, YAML, env) must be translated into this struct.
type Config struct {
	URLs       []string
	Wordlist   string
	Extensions []string

	Threads        int
	Timeout        time.Duration
	Delay          time.Duration
	Rate           float64
	ConnectTimeout time.Duration

	Recursive       bool
	MaxDepth        uint16
	Strategy        recursion.Strategy
	FollowRedirects bool
	OnlyRedirects   bool
	MaxRedirects    int

	RecurseOn status.Filters

	Paths PathConfig

	OutputFile   string // "" means write to stdout
	OutputFormat string // "text", "json", "ndjson", "csv", "markdown"
	Quiet        bool

	IncludeSize size.Filters
	ExcludeSize size.Filters

	Status StatusConfig

	// Request Manipulation fields
	Method      string
	HTTPVersion string
	Data        string
	Headers     []string
	Cookies     string
	Proxy       string
	Insecure    bool // Allow insecure TLS connections (skip TLS certificate verification)

	// Response Filtering fields
	MatchRegex    []string
	FilterRegex   []string
	MatchContent  []string
	FilterContent []string

	// Adaptive enables technology detection and adaptive path injection.
	Adaptive bool

	// Response Presentation fields
	ShowHeaders        bool
	ShowTitle          bool
	HumanReadableSizes bool

	// HTTP Request Templates
	RequestFile string

	// FuzzStrategy determines the hierarchical traversal strategy for fuzzing
	FuzzStrategy string

	// User-Agent management.
	// UserAgent is the value supplied via --user-agent.
	// RandomAgent is true when --random-agent is passed or a profile sets random-agent: true.
	// Resolution order is enforced by internal/useragent.Resolve.
	UserAgent   string
	RandomAgent bool
}

type PathConfig struct {
	NormalizePaths  bool
	CollapseSlashes bool
}

type StatusConfig struct {
	Include status.Filters
	Exclude status.Filters
}
