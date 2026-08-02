package fuzz

import (
	"github.com/unsubble/searchit/internal/config"
)

// Apply merges non-nil fields from the overlay into cfg.
func Apply(cfg *config.Config, o Overlay) {
	config.ApplyFuzzOverlay(cfg, o)
}
