package color

import (
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/term"
)

const (
	// ModeAuto enables colors when output is connected to an interactive terminal
	// and quiet mode is not active.
	ModeAuto = "auto"
	// ModeAlways forces colors regardless of output target or quiet mode.
	ModeAlways = "always"
	// ModeNever disables all ANSI color codes.
	ModeNever = "never"
)

const (
	ansiReset      = "\033[0m"
	ansiBold       = "\033[1m"
	ansiRed        = "\033[31m"
	ansiGreen      = "\033[32m"
	ansiYellow     = "\033[33m"
	ansiWhite      = "\033[37m"
	ansiBoldRed    = "\033[1;31m"
	ansiBoldGreen  = "\033[1;32m"
	ansiBoldYellow = "\033[1;33m"
	ansiBoldWhite  = "\033[1;37m"
)

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

var validModes = map[string]struct{}{
	ModeAuto:   {},
	ModeAlways: {},
	ModeNever:  {},
}

// IsValidMode returns true if mode is a recognized color mode ("auto", "always", "never").
func IsValidMode(mode string) bool {
	_, ok := validModes[strings.ToLower(strings.TrimSpace(mode))]
	return ok
}

// Resolve determines whether colors should be enabled based on mode, quiet flag, and TTY status.
//
// Precedence:
//  1. mode == "always": colors are always enabled, even in quiet mode and even when redirected.
//  2. mode == "never": colors are always disabled.
//  3. mode == "auto" (or empty):
//     - if quiet is true: colors are disabled.
//     - if quiet is false: colors depend on TTY detector (enabled if TTY, disabled if not).
//  4. any other mode: returns false.
func Resolve(mode string, quiet bool, isTTY func() bool) bool {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case ModeAlways:
		return true
	case ModeNever:
		return false
	case ModeAuto, "":
		if quiet {
			return false
		}
		if isTTY != nil {
			return isTTY()
		}
		return defaultIsTTY()
	default:
		return false
	}
}

// Palette provides semantic styling helpers.
type Palette struct {
	enabled bool
}

// New creates a new Palette with the given color enabled state.
func New(enabled bool) *Palette {
	return &Palette{enabled: enabled}
}

// Enabled reports whether styling is enabled.
func (p *Palette) Enabled() bool {
	return p.enabled
}

// Success styles a string in bold green (successful findings / positive status).
func (p *Palette) Success(s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return ansiBoldGreen + s + ansiReset
}

// Warning styles a string in bold yellow (warnings and noteworthy conditions).
func (p *Palette) Warning(s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return ansiBoldYellow + s + ansiReset
}

// Error styles a string in bold red (errors and failures).
func (p *Palette) Error(s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return ansiBoldRed + s + ansiReset
}

// Status styles an HTTP status code with bold semantic color:
// - 2xx: Bold Green (successful)
// - 3xx: Bold Yellow (redirect / warning)
// - 4xx: Bold Yellow (client error / noteworthy)
// - 5xx: Bold Red (server error / failure)
// - other: plain representation
func (p *Palette) Status(code int) string {
	codeStr := strconv.Itoa(code)
	if !p.enabled {
		return codeStr
	}
	switch {
	case code >= 200 && code < 300:
		return ansiBoldGreen + codeStr + ansiReset
	case code >= 300 && code < 500:
		return ansiBoldYellow + codeStr + ansiReset
	case code >= 500 && code < 600:
		return ansiBoldRed + codeStr + ansiReset
	default:
		return codeStr
	}
}

// StatusBracketed styles a bracketed HTTP status code (e.g. "[301]") with bold semantic color.
// The brackets and status code are both styled together.
func (p *Palette) StatusBracketed(code int) string {
	s := "[" + strconv.Itoa(code) + "]"
	if !p.enabled {
		return s
	}
	switch {
	case code >= 200 && code < 300:
		return ansiBoldGreen + s + ansiReset
	case code >= 300 && code < 500:
		return ansiBoldYellow + s + ansiReset
	case code >= 500 && code < 600:
		return ansiBoldRed + s + ansiReset
	default:
		return s
	}
}

// Bold styles a string in bold with default terminal color.
func (p *Palette) Bold(s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return ansiBold + s + ansiReset
}

// Heading styles a CLI section heading (bold with default terminal color).
func (p *Palette) Heading(s string) string {
	return p.Bold(s)
}

// Finding styles a finding result (e.g. found URL in quiet mode when colors are enabled).
// URLs remain at normal weight in accordance with styling rules.
func (p *Palette) Finding(s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return ansiGreen + s + ansiReset
}

// BoldWhite styles a string in bold white.
func (p *Palette) BoldWhite(s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return ansiBoldWhite + s + ansiReset
}

// ByteReceived styles the byte received label in bold white.
// Keeps the unit label white regardless of semantic colors used elsewhere.
func (p *Palette) ByteReceived(s string) string {
	return p.BoldWhite(s)
}

// PlainSuccess styles a string in normal-weight green.
func (p *Palette) PlainSuccess(s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return ansiGreen + s + ansiReset
}

// PlainWarning styles a string in normal-weight yellow.
func (p *Palette) PlainWarning(s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return ansiYellow + s + ansiReset
}

// PlainError styles a string in normal-weight red.
func (p *Palette) PlainError(s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return ansiRed + s + ansiReset
}

// Global palette and package-level state
var (
	globalPalette = New(false)
	globalMu      sync.RWMutex

	ttyMu       sync.RWMutex
	ttyDetector func() bool
)

// Configure sets the global color palette based on mode, quiet flag, and TTY detector.
func Configure(mode string, quiet bool, isTTY func() bool) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalPalette = New(Resolve(mode, quiet, isTTY))
}

// SetGlobalPalette sets the global palette directly.
func SetGlobalPalette(p *Palette) {
	globalMu.Lock()
	defer globalMu.Unlock()
	if p == nil {
		globalPalette = New(false)
	} else {
		globalPalette = p
	}
}

// Global returns the current global Palette.
func Global() *Palette {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalPalette
}

// Package-level semantic helpers using the global palette
func Success(s string) string         { return Global().Success(s) }
func Warning(s string) string         { return Global().Warning(s) }
func Error(s string) string           { return Global().Error(s) }
func Status(code int) string          { return Global().Status(code) }
func StatusBracketed(code int) string { return Global().StatusBracketed(code) }
func Finding(s string) string         { return Global().Finding(s) }
func Bold(s string) string            { return Global().Bold(s) }
func Heading(s string) string         { return Global().Heading(s) }
func BoldWhite(s string) string       { return Global().BoldWhite(s) }
func ByteReceived(s string) string    { return Global().ByteReceived(s) }
func PlainSuccess(s string) string    { return Global().PlainSuccess(s) }
func PlainWarning(s string) string    { return Global().PlainWarning(s) }
func PlainError(s string) string      { return Global().PlainError(s) }
func IsEnabled() bool                 { return Global().Enabled() }

// SetTTYDetector sets an injectable TTY detection function, primarily for testing.
// Pass nil to restore default behavior.
func SetTTYDetector(fn func() bool) {
	ttyMu.Lock()
	defer ttyMu.Unlock()
	ttyDetector = fn
}

// ResetTTYDetector restores the default TTY detector.
func ResetTTYDetector() {
	SetTTYDetector(nil)
}

func defaultIsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// IsTerminal checks if the given writer is an interactive terminal.
// If a custom detector was registered via SetTTYDetector, it takes precedence.
func IsTerminal(w io.Writer) bool {
	ttyMu.RLock()
	custom := ttyDetector
	ttyMu.RUnlock()
	if custom != nil {
		return custom()
	}
	if w == nil {
		return defaultIsTTY()
	}
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// Strip removes all ANSI escape codes from s.
func Strip(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

// HasANSI reports whether s contains any ANSI escape codes.
func HasANSI(s string) bool {
	return ansiRegexp.MatchString(s)
}

// HasBold reports whether s contains any ANSI bold escape codes (e.g. \033[1m or \033[1;...m).
func HasBold(s string) bool {
	return strings.Contains(s, "\033[1;") || strings.Contains(s, "\033[1m")
}

// HasWhite reports whether s contains any ANSI white escape codes (e.g. \033[37m or \033[1;37m).
func HasWhite(s string) bool {
	return strings.Contains(s, "\033[37m") || strings.Contains(s, "\033[1;37m")
}
