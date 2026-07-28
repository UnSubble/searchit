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
	// Target
	URL     *string `yaml:"url"`
	URLFile *string `yaml:"url-file"`

	// Wordlist & extensions
	Wordlist   *string   `yaml:"wordlist"`
	Extensions *[]string `yaml:"ext"`

	// Concurrency & timing
	Threads        *int           `yaml:"threads"`
	Timeout        *time.Duration `yaml:"timeout"`
	ConnectTimeout *time.Duration `yaml:"connect-timeout"`
	Delay          *time.Duration `yaml:"delay"`
	Rate           *float64       `yaml:"rate"`

	// Recursion
	Recursive *bool   `yaml:"recursive"`
	MaxDepth  *uint16 `yaml:"max-depth"`
	Strategy  *string `yaml:"strategy"`
	RecurseOn *string `yaml:"recurse-on"`

	// Path normalisation
	NormalizePaths  *bool `yaml:"normalize-paths"`
	CollapseSlashes *bool `yaml:"collapse-slashes"`

	// Output
	Format *string `yaml:"format"`
	// Output is a deprecated alias for Format. If Format is absent, Output is used.
	Output   *string `yaml:"output"`
	Quiet    *bool   `yaml:"quiet"`
	LogCount *int    `yaml:"log-count"`

	// Redirects
	FollowRedirects *bool `yaml:"follow-redirects"`
	MaxRedirects    *int  `yaml:"max-redirects"`

	// Status filtering
	ExcludeStatus *string `yaml:"exclude-status"`
	MatchStatus   *string `yaml:"match-status"`

	// Size filtering
	IncludeSize *string `yaml:"include-size"`
	ExcludeSize *string `yaml:"exclude-size"`

	// Response-header filtering
	IncludeHeaders *[]string `yaml:"include-headers"`
	ExcludeHeaders *[]string `yaml:"exclude-headers"`

	// Regex / content filtering
	MatchRegex    *[]string `yaml:"match-regex"`
	FilterRegex   *[]string `yaml:"filter-regex"`
	MatchContent  *[]string `yaml:"match-content"`
	FilterContent *[]string `yaml:"filter-content"`

	// HTTP request manipulation
	Method  *string   `yaml:"method"`
	Data    *string   `yaml:"data"`
	Headers *[]string `yaml:"headers"`
	Cookies *string   `yaml:"cookies"`
	Proxy   *string   `yaml:"proxy"`
	Request *string   `yaml:"request"`

	// Adaptive & presentation
	Adaptive    *bool `yaml:"adaptive"`
	ShowHeaders *bool `yaml:"show-headers"`
	ShowTitle   *bool `yaml:"show-title"`

	// User-Agent
	UserAgent   *string `yaml:"user-agent"`
	RandomAgent *bool   `yaml:"random-agent"`
}

// UnmarshalYAML implements custom unmarshaling to handle durations gracefully.
// Durations in YAML can be represented as strings (e.g. "10s", "100ms") or integers.
func (o *Overlay) UnmarshalYAML(value *yaml.Node) error {
	type rawOverlay struct {
		URL     *string `yaml:"url"`
		URLFile *string `yaml:"url-file"`

		Wordlist   *string   `yaml:"wordlist"`
		Extensions *[]string `yaml:"ext"`

		Threads        *int      `yaml:"threads"`
		Timeout        yaml.Node `yaml:"timeout"`
		ConnectTimeout yaml.Node `yaml:"connect-timeout"`
		Delay          yaml.Node `yaml:"delay"`
		Rate           *float64  `yaml:"rate"`

		Recursive *bool   `yaml:"recursive"`
		MaxDepth  *uint16 `yaml:"max-depth"`
		Strategy  *string `yaml:"strategy"`
		RecurseOn *string `yaml:"recurse-on"`

		NormalizePaths  *bool `yaml:"normalize-paths"`
		CollapseSlashes *bool `yaml:"collapse-slashes"`

		Format   *string `yaml:"format"`
		Output   *string `yaml:"output"`
		Quiet    *bool   `yaml:"quiet"`
		LogCount *int    `yaml:"log-count"`

		FollowRedirects *bool `yaml:"follow-redirects"`
		MaxRedirects    *int  `yaml:"max-redirects"`

		ExcludeStatus *string `yaml:"exclude-status"`
		MatchStatus   *string `yaml:"match-status"`

		IncludeSize *string `yaml:"include-size"`
		ExcludeSize *string `yaml:"exclude-size"`

		IncludeHeaders *[]string `yaml:"include-headers"`
		IncludeHeader  *[]string `yaml:"include-header"`
		ExcludeHeaders *[]string `yaml:"exclude-headers"`
		ExcludeHeader  *[]string `yaml:"exclude-header"`

		MatchRegex    *[]string `yaml:"match-regex"`
		FilterRegex   *[]string `yaml:"filter-regex"`
		MatchContent  *[]string `yaml:"match-content"`
		FilterContent *[]string `yaml:"filter-content"`

		Method  *string   `yaml:"method"`
		Data    *string   `yaml:"data"`
		Headers *[]string `yaml:"headers"`
		Cookies *string   `yaml:"cookies"`
		Proxy   *string   `yaml:"proxy"`
		Request *string   `yaml:"request"`

		Adaptive    *bool `yaml:"adaptive"`
		ShowHeaders *bool `yaml:"show-headers"`
		ShowTitle   *bool `yaml:"show-title"`

		UserAgent   *string `yaml:"user-agent"`
		RandomAgent *bool   `yaml:"random-agent"`
	}

	var raw rawOverlay
	if err := value.Decode(&raw); err != nil {
		return err
	}

	// Target
	o.URL = raw.URL
	o.URLFile = raw.URLFile

	// Wordlist & extensions
	o.Wordlist = raw.Wordlist
	o.Extensions = raw.Extensions

	// Concurrency & timing (non-duration fields)
	o.Threads = raw.Threads
	o.Rate = raw.Rate

	// Recursion
	o.Recursive = raw.Recursive
	o.MaxDepth = raw.MaxDepth
	o.Strategy = raw.Strategy
	o.RecurseOn = raw.RecurseOn

	// Path normalisation
	o.NormalizePaths = raw.NormalizePaths
	o.CollapseSlashes = raw.CollapseSlashes

	// Output — prefer canonical "format"; fall back to deprecated "output".
	if raw.Format != nil {
		o.Format = raw.Format
	} else {
		o.Format = raw.Output
	}
	o.Output = raw.Output
	o.Quiet = raw.Quiet
	o.LogCount = raw.LogCount

	// Redirects
	o.FollowRedirects = raw.FollowRedirects
	o.MaxRedirects = raw.MaxRedirects

	// Status filtering
	o.ExcludeStatus = raw.ExcludeStatus
	o.MatchStatus = raw.MatchStatus

	// Size filtering
	o.IncludeSize = raw.IncludeSize
	o.ExcludeSize = raw.ExcludeSize

	// Regex / content filtering
	o.MatchRegex = raw.MatchRegex
	o.FilterRegex = raw.FilterRegex
	o.MatchContent = raw.MatchContent
	o.FilterContent = raw.FilterContent

	// HTTP request manipulation
	o.Method = raw.Method
	o.Data = raw.Data
	o.Headers = raw.Headers
	o.Cookies = raw.Cookies
	o.Proxy = raw.Proxy
	o.Request = raw.Request

	// Adaptive & presentation
	o.Adaptive = raw.Adaptive
	o.ShowHeaders = raw.ShowHeaders
	o.ShowTitle = raw.ShowTitle

	// User-Agent
	o.UserAgent = raw.UserAgent
	o.RandomAgent = raw.RandomAgent

	// Header aliases: prefer plural form.
	if raw.IncludeHeaders != nil {
		o.IncludeHeaders = raw.IncludeHeaders
	} else {
		o.IncludeHeaders = raw.IncludeHeader
	}
	if raw.ExcludeHeaders != nil {
		o.ExcludeHeaders = raw.ExcludeHeaders
	} else {
		o.ExcludeHeaders = raw.ExcludeHeader
	}

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
