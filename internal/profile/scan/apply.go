package scan

import (
	"regexp"
	"strings"

	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/extensions"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
)

// Apply merges non-nil fields from the overlay into cfg.
// Only explicitly present fields overwrite existing values.
func Apply(cfg *config.Config, o Overlay) {
	// Target
	if o.URL != nil {
		cfg.URLs = []string{*o.URL}
	}
	if o.URLFile != nil {
		cfg.URLFile = *o.URLFile
	}

	// Wordlist & extensions
	if o.Wordlist != nil {
		cfg.Wordlist = *o.Wordlist
	}
	if o.Extensions != nil {
		if exts, err := extensions.Parse(*o.Extensions); err == nil {
			cfg.Extensions = exts
		}
	}

	// Concurrency & timing
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

	// Recursion
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

	// Path normalisation
	if o.NormalizePaths != nil {
		cfg.Paths.NormalizePaths = *o.NormalizePaths
	}
	if o.CollapseSlashes != nil {
		cfg.Paths.CollapseSlashes = *o.CollapseSlashes
	}

	// Output
	if o.Format != nil {
		cfg.OutputFormat = *o.Format
	}
	if o.Quiet != nil {
		cfg.Quiet = *o.Quiet
	}

	// Redirects
	if o.FollowRedirects != nil {
		cfg.FollowRedirects = *o.FollowRedirects
	}
	if o.MaxRedirects != nil {
		cfg.MaxRedirects = *o.MaxRedirects
	}

	// Status filtering
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

	// Size filtering
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

	// Response-header filtering
	if o.IncludeHeaders != nil {
		cfg.IncludeHeaders = parseHeaderFlags(*o.IncludeHeaders)
	}
	if o.ExcludeHeaders != nil {
		cfg.ExcludeHeaders = parseHeaderFlags(*o.ExcludeHeaders)
	}

	// Regex / content filtering
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

	// HTTP request manipulation
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

	// Adaptive & presentation
	if o.Adaptive != nil {
		cfg.Adaptive = *o.Adaptive
	}
	if o.ShowHeaders != nil {
		cfg.ShowHeaders = *o.ShowHeaders
	}
	if o.ShowTitle != nil {
		cfg.ShowTitle = *o.ShowTitle
	}

	// User-Agent
	if o.UserAgent != nil {
		cfg.UserAgent = *o.UserAgent
	}
	if o.RandomAgent != nil {
		cfg.RandomAgent = *o.RandomAgent
	}
}

func parseHeaderFlags(flags []string) []config.HeaderFilter {
	res := make([]config.HeaderFilter, 0, len(flags))
	for _, f := range flags {
		idx := strings.Index(f, "=")
		if idx > 0 && idx < len(f)-1 {
			res = append(res, config.HeaderFilter{
				Name:  f[:idx],
				Value: f[idx+1:],
			})
		}
	}
	return res
}
