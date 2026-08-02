// Package importer converts Searchit CLI command strings into profile-compatible
// YAML configuration nodes. It supports both scan and fuzz commands.
//
// ParseCommand is the primary entry point. It tokenizes the command string,
// strips the optional executable prefix, validates the subcommand against
// the expected tool, and builds a yaml.Node MappingNode from every flag that
// the user explicitly set.
//
// Flag registration is shared with the live CLI via flagreg.RegisterScanFlags /
// flagreg.RegisterFuzzFlags. Adding a new profile-supported flag to those
// registration functions automatically makes it importable here — no importer
// changes required.
//
// The only importer-specific knowledge is:
//
//  1. cliToYAML — the ~12 flags whose CLI name differs from their YAML key
//     (e.g. "header" → "headers", "mc" → "match-status"). Identical for scan
//     and fuzz: both tools share the same CLI flag naming conventions.
//     All other flags use the CLI flag name as the YAML key directly.
//
//  2. aliasFlags — the 3 alias flags (fc, ms, fs) that override their lower-
//     priority counterparts when both appear on the same command. Same for
//     both tools.
//
//  3. explicitRuntimeFlags — the 1 entry per tool that
//     must be excluded even though its YAML key exists in the overlay struct.
//
//  4. runtimeWarnMsgScan / runtimeWarnMsgFuzz — optional human-readable notes
//     for known runtime-only flags. These differ between tools.
//     Whether a flag is runtime-only is determined automatically by checking
//     the overlay struct's YAML tags via reflection.
package importer

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
	fuzzOverlay "github.com/unsubble/searchit/internal/profile/fuzz"
	scanOverlay "github.com/unsubble/searchit/internal/profile/scan"
	"gopkg.in/yaml.v3"
)

// ── Importer-specific tables (the irreducible minimum) ─────────────────────

// cliToYAML maps CLI flag names to their YAML key for every flag where the
// two names differ. Flags absent from this map use the CLI flag name directly
// as the YAML key (the common case).
//
// Scan and fuzz share identical naming conventions, so a single map covers
// both tools. If a future tool introduces a divergence, split this back into
// tool-specific maps at that point.
//
// The map also participates in alias priority: when two flags resolve to the
// same YAML key (e.g. "fc" and "exclude-status"), the flag present in
// aliasFlags wins regardless of command-line order.
var cliToYAML = map[string]string{
	// Plural/singular rename (CLI is singular, YAML is plural)
	"header":         "headers",
	"cookie":         "cookies",
	"include-header": "include-headers",
	"exclude-header": "exclude-headers",
	// Short-form aliases that map to a different YAML key than their long name
	"mc": "match-status",
	"fc": "exclude-status", // alias for exclude-status with higher priority
	"ms": "include-size",   // alias for include-size with higher priority
	"fs": "exclude-size",   // alias for exclude-size with higher priority
	"mr": "match-regex",
	"fr": "filter-regex",
	"mt": "match-content",
	"ft": "filter-content",
}

// aliasFlags is the set of flags that take priority when two CLI flags resolve
// to the same YAML key. Identical for scan and fuzz.
var aliasFlags = map[string]bool{
	"fc": true, // beats exclude-status
	"ms": true, // beats include-size
	"fs": true, // beats exclude-size
}

// explicitRuntimeFlags is the set of CLI flags that are always runtime-only
// even if their YAML key exists in the overlay struct. This covers the edge case
// where a CLI flag name matches a deprecated overlay field with different semantics
// (e.g. --output is a file path, but overlay's "output" key is a deprecated format alias).
//
// Identical for both scan and fuzz as they share the same overarching behavior for
// these specific flags.
var explicitRuntimeFlags = map[string]bool{
	"output":      true,
	"no-progress": true,
	"foo":         true,
	"bar":         true,
	"buzz":        true,
}

// runtimeWarnMsgScan provides human-readable notes for scan flags that have
// no overlay equivalent. These are shown as informational stderr notes.
// Flags not in this map but absent from the overlay are silently skipped.
var runtimeWarnMsgScan = map[string]string{
	"profile":     "--profile cannot be imported: profiles cannot recursively reference other profiles during creation",
	"no-progress": "--no-progress is a runtime display flag and cannot be saved in a profile",
	"tech":        "--tech is runtime-only; technology detection cannot be saved in a profile",
	"output":      "--output sets an output file path and cannot be saved in a profile; use --format to set the output format",
}

// runtimeWarnMsgFuzz provides human-readable notes for fuzz runtime-only flags.
var runtimeWarnMsgFuzz = map[string]string{
	"profile":     "--profile cannot be imported: profiles cannot recursively reference other profiles during creation",
	"no-progress": "--no-progress is a runtime display flag and cannot be saved in a profile",
	"output":      "--output sets an output file path and cannot be saved in a profile; use --format to set the output format",
	"foo":         "--foo is a wordlist placeholder and cannot be saved in a profile",
	"bar":         "--bar is a wordlist placeholder and cannot be saved in a profile",
	"buzz":        "--buzz is a wordlist placeholder and cannot be saved in a profile",
}

// ── Overlay YAML key sets (auto-derived via reflection) ───────────────────

// overlayKeysScan is the set of all YAML keys present in scan.Overlay.
// A CLI flag is profile-supported iff its resolved YAML key is in this set.
// Built once at init time.
var overlayKeysScan = buildOverlayKeys(scanOverlay.Overlay{})

// overlayKeysFuzz is the same for fuzz.Overlay.
var overlayKeysFuzz = buildOverlayKeys(fuzzOverlay.Overlay{})

// buildOverlayKeys extracts all yaml struct tags from v's type via reflection.
func buildOverlayKeys(v any) map[string]bool {
	t := reflect.TypeOf(v)
	keys := make(map[string]bool, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		key, _, _ := strings.Cut(tag, ",")
		if key != "" {
			keys[key] = true
		}
	}
	return keys
}

// ── Public API ────────────────────────────────────────────────────────────

// ParseCommand converts a Searchit command string into a profile-compatible
// yaml.Node config section.
//
// tool must be "scan" or "fuzz". cmdStr is the full command string, e.g.:
//
//	"searchit scan -t 128 --adaptive --random-agent"
//	"fuzz --strategy eager --threads 64"
//
// Returns:
//   - configNode: a yaml.MappingNode suitable for profile.Profile.Config
//   - warnings:   informational messages for runtime-only flags that were present
//   - err:        non-nil for malformed input or namespace mismatches
func ParseCommand(tool, cmdStr string, fsFactory func() *pflag.FlagSet) (yaml.Node, []string, error) {
	tokens, err := Tokenize(cmdStr)
	if err != nil {
		return yaml.Node{}, nil, fmt.Errorf("malformed command: %w", err)
	}
	if len(tokens) == 0 {
		return yaml.Node{}, nil, fmt.Errorf("command string is empty")
	}

	tokens = stripExecutable(tokens)
	if len(tokens) == 0 {
		return yaml.Node{}, nil, fmt.Errorf("command must include a subcommand (scan or fuzz)")
	}

	subcommand := strings.ToLower(tokens[0])
	if subcommand != tool {
		return yaml.Node{}, nil, fmt.Errorf(
			"command %q does not match profile tool %q\n"+
				"  a %s/... profile requires a %q command",
			subcommand, tool, tool, tool,
		)
	}
	tokens = tokens[1:]

	switch tool {
	case "scan":
		return parseScan(tokens, fsFactory)
	case "fuzz":
		return parseFuzz(tokens, fsFactory)
	default:
		return yaml.Node{}, nil, fmt.Errorf("unsupported tool %q: must be scan or fuzz", tool)
	}
}

// Tokenize splits a command string into tokens using shell-like quoting rules.
// It supports single quotes, double quotes, and backslash escaping.
func Tokenize(s string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == '\\' && !inSingle:
			if i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
			}
		case (c == ' ' || c == '\t' || c == '\n') && !inSingle && !inDouble:
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}

	if inSingle {
		return nil, fmt.Errorf("unterminated single quote")
	}
	if inDouble {
		return nil, fmt.Errorf("unterminated double quote")
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens, nil
}

// ── Internal: per-tool parse functions ───────────────────────────────────

func parseScan(tokens []string, fsFactory func() *pflag.FlagSet) (yaml.Node, []string, error) {
	fs := fsFactory()
	fs.Init("scan", pflag.ContinueOnError)

	if err := fs.Parse(tokens); err != nil {
		return yaml.Node{}, nil, fmt.Errorf("unrecognised flag in command: %w", err)
	}

	return buildConfig(fs, cliToYAML, aliasFlags, overlayKeysScan, explicitRuntimeFlags, runtimeWarnMsgScan)
}

func parseFuzz(tokens []string, fsFactory func() *pflag.FlagSet) (yaml.Node, []string, error) {
	fs := fsFactory()
	fs.Init("fuzz", pflag.ContinueOnError)

	if err := fs.Parse(tokens); err != nil {
		return yaml.Node{}, nil, fmt.Errorf("unrecognised flag in command: %w", err)
	}

	return buildConfig(fs, cliToYAML, aliasFlags, overlayKeysFuzz, explicitRuntimeFlags, runtimeWarnMsgFuzz)
}

// ── Config node builder ───────────────────────────────────────────────────

// winner records the winning yaml.Node for a YAML key and whether it came from
// an alias (which has higher priority).
type winner struct {
	node    *yaml.Node
	isAlias bool
}

// buildConfig iterates over every Changed flag in fs and produces a
// yaml.MappingNode containing only the profile-supported fields.
//
// Priority rule: when two flags resolve to the same YAML key and one of them
// is an alias (present in aliases), the alias value wins regardless of order.
//
// explicitRuntime contains CLI flag names that are always runtime-only even when
// their resolved YAML key appears in the overlay struct (e.g. --output whose YAML
// key "output" exists as a deprecated overlay field with different semantics).
func buildConfig(
	fs *pflag.FlagSet,
	cliToYAML map[string]string,
	aliases map[string]bool,
	overlayKeys map[string]bool,
	explicitRuntime map[string]bool,
	warnMsgs map[string]string,
) (yaml.Node, []string, error) {
	winners := make(map[string]winner)
	var warnings []string

	// The canonical ordering of YAML keys is determined by the order we visit flags.
	// We preserve insertion order using an ordered key list.
	var orderedKeys []string

	fs.VisitAll(func(f *pflag.Flag) {
		if !f.Changed {
			return
		}

		// Resolve the YAML key for this flag.
		yamlKey, ok := cliToYAML[f.Name]
		if !ok {
			yamlKey = f.Name // CLI name == YAML key (the common case)
		}

		// Check if this flag is runtime-only. A flag is runtime-only if:
		//  a) its YAML key is not in the overlay struct, OR
		//  b) it is in the explicit exclusion set (overlay key name collision).
		if !overlayKeys[yamlKey] || explicitRuntime[f.Name] {
			if msg, known := warnMsgs[f.Name]; known {
				warnings = append(warnings, msg)
			}
			return
		}

		// Build the yaml.Node value for this flag.
		node, err := nodeFromFlag(f)
		if err != nil {
			return
		}

		existing, exists := winners[yamlKey]
		isAlias := aliases[f.Name]

		if !exists {
			winners[yamlKey] = winner{node: node, isAlias: isAlias}
			orderedKeys = append(orderedKeys, yamlKey)
		} else if isAlias && !existing.isAlias {
			// Alias beats the non-alias for the same YAML key.
			winners[yamlKey] = winner{node: node, isAlias: true}
		}
		// Non-alias does not override alias; same-priority keeps first-seen.
	})

	// Deduplicate orderedKeys (a key may appear twice if alias + primary both set).
	seen := make(map[string]bool, len(orderedKeys))
	configNode := yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, key := range orderedKeys {
		if seen[key] {
			continue
		}
		seen[key] = true
		w := winners[key]
		configNode.Content = append(configNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"},
			w.node,
		)
	}

	return configNode, warnings, nil
}

// nodeFromFlag converts a pflag.Flag's current Value into a yaml.Node with the
// correct YAML tag. It uses the pflag type name to select the right tag.
func nodeFromFlag(f *pflag.Flag) (*yaml.Node, error) {
	switch f.Value.Type() {
	case "string":
		return &yaml.Node{Kind: yaml.ScalarNode, Value: f.Value.String(), Tag: "!!str"}, nil

	case "int", "int8", "int16", "int32", "int64":
		return &yaml.Node{Kind: yaml.ScalarNode, Value: f.Value.String(), Tag: "!!int"}, nil

	case "uint", "uint8", "uint16", "uint32", "uint64":
		return &yaml.Node{Kind: yaml.ScalarNode, Value: f.Value.String(), Tag: "!!int"}, nil

	case "float32", "float64":
		// pflag serialises float64 as e.g. "1.5" or "10"; use that directly.
		return &yaml.Node{Kind: yaml.ScalarNode, Value: f.Value.String(), Tag: "!!float"}, nil

	case "bool":
		b, err := strconv.ParseBool(f.Value.String())
		if err != nil {
			return nil, err
		}
		bStr := "false"
		if b {
			bStr = "true"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Value: bStr, Tag: "!!bool"}, nil

	case "stringSlice", "stringArray":
		sliceVal, ok := f.Value.(pflag.SliceValue)
		if !ok {
			return nil, fmt.Errorf("flag %q is %s but does not implement pflag.SliceValue", f.Name, f.Value.Type())
		}
		items := sliceVal.GetSlice()
		if len(items) == 0 {
			return nil, fmt.Errorf("empty slice for flag %q", f.Name)
		}
		seqNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range items {
			seqNode.Content = append(seqNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: item, Tag: "!!str"},
			)
		}
		return seqNode, nil

	default:
		// For any unknown type (e.g. custom pflag.Value implementations),
		// fall back to a string representation.
		return &yaml.Node{Kind: yaml.ScalarNode, Value: f.Value.String(), Tag: "!!str"}, nil
	}
}

// ── Internal: token helpers ───────────────────────────────────────────────

// stripExecutable removes the optional executable prefix from tokens.
// Recognises "searchit", "searchit.exe", "./searchit", and absolute paths.
func stripExecutable(tokens []string) []string {
	if len(tokens) == 0 {
		return tokens
	}
	base := filepath.Base(tokens[0])
	base = strings.TrimSuffix(strings.ToLower(base), ".exe")
	if base == "searchit" {
		return tokens[1:]
	}
	return tokens
}
