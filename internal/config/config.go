package config

import (
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
)

// Config is the single source of truth consumed by the engine.
// All external inputs (CLI, YAML, env) must be translated into this struct.
type Config struct {
	URL      string
	Wordlist string

	Threads int
	Timeout int // seconds

	Recursive bool
	MaxDepth  uint16
	Strategy  recursion.Strategy

	RecurseOn status.Filters

	Paths PathConfig

	Output string

	IncludeSize size.Filters
	ExcludeSize size.Filters

	IncludeHeaders []HeaderFilter
	ExcludeHeaders []HeaderFilter

	Status StatusConfig
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
