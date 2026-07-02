package config

import (
	"github.com/unsubble/searchit/internal/recursion"
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

	Status StatusConfig
}

type StatusConfig struct {
	Include status.Filters
	Exclude status.Filters
}
