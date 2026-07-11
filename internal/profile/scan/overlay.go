package scan

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Overlay represents a partial scan configuration overlay.
// Pointer fields distinguish "not present" (nil) from zero values.
// Only non-nil fields are applied to the target Config.
type Overlay struct {
	Wordlist        *string        `yaml:"wordlist"`
	Threads         *int           `yaml:"threads"`
	Timeout         *time.Duration `yaml:"timeout"`
	ConnectTimeout  *time.Duration `yaml:"connect-timeout"`
	Recursive       *bool          `yaml:"recursive"`
	MaxDepth        *uint16        `yaml:"max-depth"`
	Strategy        *string        `yaml:"strategy"`
	Delay           *time.Duration `yaml:"delay"`
	Rate            *float64       `yaml:"rate"`
	Output          *string        `yaml:"output"`
	Quiet           *bool          `yaml:"quiet"`
	NormalizePaths  *bool          `yaml:"normalize-paths"`
	CollapseSlashes *bool          `yaml:"collapse-slashes"`
	ExcludeStatus   *string        `yaml:"exclude-status"`
	RecurseOn       *string        `yaml:"recurse-on"`
	IncludeSize     *string        `yaml:"include-size"`
	ExcludeSize     *string        `yaml:"exclude-size"`
	IncludeHeaders  *[]string      `yaml:"include-headers"`
	ExcludeHeaders  *[]string      `yaml:"exclude-headers"`
}

// UnmarshalYAML implements custom unmarshaling to handle durations gracefully.
// Durations in YAML can be represented as strings (e.g. "10s", "100ms") or integers.
func (o *Overlay) UnmarshalYAML(value *yaml.Node) error {
	type rawOverlay struct {
		Wordlist        *string   `yaml:"wordlist"`
		Threads         *int      `yaml:"threads"`
		Timeout         yaml.Node `yaml:"timeout"`
		ConnectTimeout  yaml.Node `yaml:"connect-timeout"`
		Recursive       *bool     `yaml:"recursive"`
		MaxDepth        *uint16   `yaml:"max-depth"`
		Strategy        *string   `yaml:"strategy"`
		Delay           yaml.Node `yaml:"delay"`
		Rate            *float64  `yaml:"rate"`
		Output          *string   `yaml:"output"`
		Quiet           *bool     `yaml:"quiet"`
		NormalizePaths  *bool     `yaml:"normalize-paths"`
		CollapseSlashes *bool     `yaml:"collapse-slashes"`
		ExcludeStatus   *string   `yaml:"exclude-status"`
		RecurseOn       *string   `yaml:"recurse-on"`
		IncludeSize     *string   `yaml:"include-size"`
		ExcludeSize     *string   `yaml:"exclude-size"`
		IncludeHeaders  *[]string `yaml:"include-headers"`
		ExcludeHeaders  *[]string `yaml:"exclude-headers"`
	}

	var raw rawOverlay
	if err := value.Decode(&raw); err != nil {
		return err
	}

	o.Wordlist = raw.Wordlist
	o.Threads = raw.Threads
	o.Recursive = raw.Recursive
	o.MaxDepth = raw.MaxDepth
	o.Strategy = raw.Strategy
	o.Rate = raw.Rate
	o.Output = raw.Output
	o.Quiet = raw.Quiet
	o.NormalizePaths = raw.NormalizePaths
	o.CollapseSlashes = raw.CollapseSlashes
	o.ExcludeStatus = raw.ExcludeStatus
	o.RecurseOn = raw.RecurseOn
	o.IncludeSize = raw.IncludeSize
	o.ExcludeSize = raw.ExcludeSize
	o.IncludeHeaders = raw.IncludeHeaders
	o.ExcludeHeaders = raw.ExcludeHeaders

	// Parse Timeout
	if raw.Timeout.Kind != 0 {
		var i int
		if err := raw.Timeout.Decode(&i); err == nil {
			d := time.Duration(i) * time.Second
			o.Timeout = &d
		} else {
			var s string
			if err := raw.Timeout.Decode(&s); err == nil {
				d, err := time.ParseDuration(s)
				if err != nil {
					return fmt.Errorf("invalid timeout duration %q: %w", s, err)
				}
				o.Timeout = &d
			} else {
				return fmt.Errorf("invalid timeout format")
			}
		}
	}

	// Parse ConnectTimeout
	if raw.ConnectTimeout.Kind != 0 {
		var i int
		if err := raw.ConnectTimeout.Decode(&i); err == nil {
			d := time.Duration(i) * time.Second
			o.ConnectTimeout = &d
		} else {
			var s string
			if err := raw.ConnectTimeout.Decode(&s); err == nil {
				d, err := time.ParseDuration(s)
				if err != nil {
					return fmt.Errorf("invalid connect-timeout duration %q: %w", s, err)
				}
				o.ConnectTimeout = &d
			} else {
				return fmt.Errorf("invalid connect-timeout format")
			}
		}
	}

	// Parse Delay
	if raw.Delay.Kind != 0 {
		var i int
		if err := raw.Delay.Decode(&i); err == nil {
			d := time.Duration(i) * time.Millisecond
			o.Delay = &d
		} else {
			var s string
			if err := raw.Delay.Decode(&s); err == nil {
				d, err := time.ParseDuration(s)
				if err != nil {
					return fmt.Errorf("invalid delay duration %q: %w", s, err)
				}
				o.Delay = &d
			} else {
				return fmt.Errorf("invalid delay format")
			}
		}
	}

	return nil
}
