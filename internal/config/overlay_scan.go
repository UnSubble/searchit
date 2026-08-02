package config

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/unsubble/searchit/internal/extensions"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
	"gopkg.in/yaml.v3"
)

// ScanOverlay represents a partial scan configuration overlay.
// Pointer fields distinguish "not present" (nil) from zero values.
type ScanOverlay struct {
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
	Output *string `yaml:"output"`
	Quiet  *bool   `yaml:"quiet"`

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

func (o *ScanOverlay) UnmarshalYAML(value *yaml.Node) error {
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

		Format     *string `yaml:"format"`
		Output     *string `yaml:"output"`
		Quiet      *bool   `yaml:"quiet"`
		NoProgress *bool   `yaml:"no-progress"`
		LogCount   *int    `yaml:"log-count"`

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
		Cookie  *string   `yaml:"cookie"`
		Proxy   *string   `yaml:"proxy"`
		Request *string   `yaml:"request"`

		Adaptive    *bool `yaml:"adaptive"`
		ShowHeaders *bool `yaml:"show-headers"`
		ShowTitle   *bool `yaml:"show-title"`

		UserAgent   *string `yaml:"user-agent"`
		RandomAgent *bool   `yaml:"random-agent"`
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err := enc.Encode(value); err != nil {
		return err
	}
	dec := yaml.NewDecoder(&buf)
	dec.KnownFields(true)

	var raw rawOverlay
	if err := dec.Decode(&raw); err != nil {
		return err
	}

	o.URL = raw.URL
	o.URLFile = raw.URLFile
	o.Wordlist = raw.Wordlist
	o.Extensions = raw.Extensions
	o.Threads = raw.Threads
	o.Rate = raw.Rate
	o.Recursive = raw.Recursive
	o.MaxDepth = raw.MaxDepth
	o.Strategy = raw.Strategy
	o.RecurseOn = raw.RecurseOn
	o.NormalizePaths = raw.NormalizePaths
	o.CollapseSlashes = raw.CollapseSlashes

	if raw.Format != nil {
		o.Format = raw.Format
	} else {
		o.Format = raw.Output
	}
	o.Output = raw.Output
	o.Quiet = raw.Quiet

	o.FollowRedirects = raw.FollowRedirects
	o.MaxRedirects = raw.MaxRedirects

	o.ExcludeStatus = raw.ExcludeStatus
	o.MatchStatus = raw.MatchStatus
	o.IncludeSize = raw.IncludeSize
	o.ExcludeSize = raw.ExcludeSize

	o.MatchRegex = raw.MatchRegex
	o.FilterRegex = raw.FilterRegex
	o.MatchContent = raw.MatchContent
	o.FilterContent = raw.FilterContent

	o.Method = raw.Method
	o.Data = raw.Data
	o.Headers = raw.Headers
	if raw.Cookies != nil {
		o.Cookies = raw.Cookies
	} else {
		o.Cookies = raw.Cookie
	}
	o.Proxy = raw.Proxy
	o.Request = raw.Request

	o.Adaptive = raw.Adaptive
	o.ShowHeaders = raw.ShowHeaders
	o.ShowTitle = raw.ShowTitle

	o.UserAgent = raw.UserAgent
	o.RandomAgent = raw.RandomAgent

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

// ApplyScanOverlay merges non-nil fields from the overlay into cfg.
func ApplyScanOverlay(cfg *Config, o ScanOverlay) {
	if o.URL != nil {
		cfg.URLs = []string{*o.URL}
	}
	if o.URLFile != nil {
		cfg.URLFile = *o.URLFile
	}
	if o.Wordlist != nil {
		cfg.Wordlist = *o.Wordlist
	}
	if o.Extensions != nil {
		if exts, err := extensions.Parse(*o.Extensions); err == nil {
			cfg.Extensions = exts
		}
	}
	if o.Threads != nil {
		cfg.Threads = *o.Threads
	}
	if o.Timeout != nil {
		cfg.Timeout = *o.Timeout
	}
	if o.ConnectTimeout != nil {
		cfg.ConnectTimeout = *o.ConnectTimeout
	}
	if o.Delay != nil {
		cfg.Delay = *o.Delay
	}
	if o.Rate != nil {
		cfg.Rate = *o.Rate
	}
	if o.Recursive != nil {
		cfg.Recursive = *o.Recursive
	}
	if o.MaxDepth != nil {
		cfg.MaxDepth = *o.MaxDepth
	}
	if o.Strategy != nil {
		if s, err := recursion.ParseStrategy(*o.Strategy); err == nil {
			cfg.Strategy = s
		}
	}
	if o.RecurseOn != nil {
		if f, err := status.Parse(*o.RecurseOn); err == nil {
			cfg.RecurseOn = f
		}
	}
	if o.NormalizePaths != nil {
		cfg.Paths.NormalizePaths = *o.NormalizePaths
	}
	if o.CollapseSlashes != nil {
		cfg.Paths.CollapseSlashes = *o.CollapseSlashes
	}
	if o.Format != nil {
		cfg.OutputFormat = *o.Format
	}
	if o.Quiet != nil {
		cfg.Quiet = *o.Quiet
	}
	if o.FollowRedirects != nil {
		cfg.FollowRedirects = *o.FollowRedirects
	}
	if o.MaxRedirects != nil {
		cfg.MaxRedirects = *o.MaxRedirects
	}
	if o.ExcludeStatus != nil {
		if f, err := status.Parse(*o.ExcludeStatus); err == nil {
			cfg.Status.Exclude = f
		}
	}
	if o.MatchStatus != nil {
		if f, err := status.Parse(*o.MatchStatus); err == nil {
			cfg.Status.Include = f
		}
	}
	if o.IncludeSize != nil {
		if f, err := size.Parse(*o.IncludeSize); err == nil {
			cfg.IncludeSize = f
		}
	}
	if o.ExcludeSize != nil {
		if f, err := size.Parse(*o.ExcludeSize); err == nil {
			cfg.ExcludeSize = f
		}
	}
	if o.IncludeHeaders != nil {
		cfg.IncludeHeaders = parseHeaderFlags(*o.IncludeHeaders)
	}
	if o.ExcludeHeaders != nil {
		cfg.ExcludeHeaders = parseHeaderFlags(*o.ExcludeHeaders)
	}
	if o.MatchRegex != nil {
		valid := make([]string, 0, len(*o.MatchRegex))
		for _, p := range *o.MatchRegex {
			if _, err := regexp.Compile(p); err == nil {
				valid = append(valid, p)
			}
		}
		cfg.MatchRegex = valid
	}
	if o.FilterRegex != nil {
		valid := make([]string, 0, len(*o.FilterRegex))
		for _, p := range *o.FilterRegex {
			if _, err := regexp.Compile(p); err == nil {
				valid = append(valid, p)
			}
		}
		cfg.FilterRegex = valid
	}
	if o.MatchContent != nil {
		cfg.MatchContent = *o.MatchContent
	}
	if o.FilterContent != nil {
		cfg.FilterContent = *o.FilterContent
	}
	if o.Method != nil {
		cfg.Method = *o.Method
	}
	if o.Data != nil {
		cfg.Data = *o.Data
	}
	if o.Headers != nil {
		cfg.Headers = *o.Headers
	}
	if o.Cookies != nil {
		cfg.Cookies = *o.Cookies
	}
	if o.Proxy != nil {
		cfg.Proxy = *o.Proxy
	}
	if o.Request != nil {
		cfg.RequestFile = *o.Request
	}
	if o.Adaptive != nil {
		cfg.Adaptive = *o.Adaptive
	}
	if o.ShowHeaders != nil {
		cfg.ShowHeaders = *o.ShowHeaders
	}
	if o.ShowTitle != nil {
		cfg.ShowTitle = *o.ShowTitle
	}
	if o.UserAgent != nil {
		cfg.UserAgent = *o.UserAgent
	}
	if o.RandomAgent != nil {
		cfg.RandomAgent = *o.RandomAgent
	}
}

func parseHeaderFlags(flags []string) []HeaderFilter {
	res := make([]HeaderFilter, 0, len(flags))
	for _, f := range flags {
		idx := strings.Index(f, "=")
		if idx > 0 && idx < len(f)-1 {
			res = append(res, HeaderFilter{
				Name:  f[:idx],
				Value: f[idx+1:],
			})
		}
	}
	return res
}
