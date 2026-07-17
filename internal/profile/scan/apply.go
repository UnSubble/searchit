package scan

import (
	"strings"

	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/recursion"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/status"
)

// Apply merges non-nil fields from the overlay into cfg.
// Only explicitly present fields overwrite existing values.
func Apply(cfg *config.Config, o Overlay) {
	if o.Wordlist != nil {
		cfg.Wordlist = *o.Wordlist
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
	if o.Delay != nil {
		cfg.Delay = *o.Delay
	}
	if o.Rate != nil {
		cfg.Rate = *o.Rate
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
	if o.NormalizePaths != nil {
		cfg.Paths.NormalizePaths = *o.NormalizePaths
	}
	if o.CollapseSlashes != nil {
		cfg.Paths.CollapseSlashes = *o.CollapseSlashes
	}
	if o.ExcludeStatus != nil {
		if f, err := status.Parse(*o.ExcludeStatus); err == nil {
			cfg.Status.Exclude = f
		}
	}
	if o.RecurseOn != nil {
		if f, err := status.Parse(*o.RecurseOn); err == nil {
			cfg.RecurseOn = f
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
	if o.Request != nil {
		cfg.RequestFile = *o.Request
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
