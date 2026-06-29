package config

import "github.com/unsubble/searchit/internal/status"

// Config is the single source of truth consumed by the engine.
// All external inputs (CLI, YAML, env) must be translated into this struct.
type Config struct {
	Targets  []string
	Wordlist string

	Threads int
	Timeout int // seconds

	Recursive RecursiveConfig
	Status    StatusConfig
	Smart     SmartConfig
}

type RecursiveConfig struct {
	Enabled bool
	Deep    bool
	Force   bool
	Max     int
}

type StatusConfig struct {
	Include status.Filters
	Exclude status.Filters
}

type SmartConfig struct {
	Enabled  bool
	Detect   bool
	Mutate   bool
	AutoExts bool
}
