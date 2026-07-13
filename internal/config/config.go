package config

import (
	"time"

	"github.com/unsubble/searchit/internal/adaptive"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
)

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

	Recursive bool
	MaxDepth  uint16
	Strategy  recursion.Strategy

	RecurseOn status.Filters

	Paths PathConfig

	Output string
	Quiet  bool

	IncludeSize size.Filters
	ExcludeSize size.Filters

	IncludeHeaders []HeaderFilter
	ExcludeHeaders []HeaderFilter

	Status StatusConfig

	// TechProfile is the explicitly-selected adaptive technology profile.
	// A nil value means no explicit selection; automatic detection applies.
	TechProfile *adaptive.TechProfile
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
