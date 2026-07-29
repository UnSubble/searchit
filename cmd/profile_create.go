package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/importer"
	"gopkg.in/yaml.v3"
)

var profileCreateCmd = &cobra.Command{
	Use:   "create <profile> [\"<searchit command>\"]",
	Short: "Create a new profile",
	Long: `Create a reusable Searchit profile.

Profiles can be created either as an empty template or imported directly
from an existing Searchit command.

USAGE

  searchit profile create <profile>
  searchit profile create <profile> "<searchit command>"

EXAMPLES

  Create an empty profile template:

    searchit profile create scan/my-profile

  Import from a scan command:

    searchit profile create scan/performance \
        "searchit scan -t 128 --adaptive --user-agent TestBot/1.0"

  Import from a fuzz command:

    searchit profile create fuzz/api \
        "searchit fuzz --strategy eager --threads 64"

  Import without the executable prefix:

    searchit profile create scan/stealth \
        "scan --delay 500ms --rate 1 --random-agent"

NOTES

  The profile namespace must match the command type: a scan/... profile
  requires a scan command, and a fuzz/... profile requires a fuzz command.

  Runtime-only flags (--no-progress, --profile, --tech, --output) are
  recognised but ignored with an informational note — they cannot be saved
  in a profile.

  Review the created profile at any time with:

    searchit profile show <profile>`,

	// DisableFlagParsing passes all arguments (including any flags) directly to
	// RunE. This lets us detect the common mistake of passing scan/fuzz flags
	// directly to "profile create" and show a helpful error message. We handle
	// --help manually in RunE.
	DisableFlagParsing: true,
	Args:               cobra.ArbitraryArgs,
	RunE:               runProfileCreate,
	SilenceUsage:       true,
}

func init() {
	profileCmd.AddCommand(profileCreateCmd)
}

func runProfileCreate(cmd *cobra.Command, args []string) error {
	// ── Handle --help / -h manually (DisableFlagParsing prevents cobra doing it) ──
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return cmd.Help()
		}
	}

	// ── Argument validation ───────────────────────────────────────────────────

	if len(args) == 0 {
		return fmt.Errorf("profile name is required\n\nUsage:\n  searchit profile create <profile>\n  searchit profile create <profile> \"<searchit command>\"")
	}

	// Detect the common mistake of passing scan/fuzz flags directly, e.g.:
	//   searchit profile create scan/test -t 128 --adaptive
	//
	// With DisableFlagParsing all raw args (including flags) arrive here.
	// We identify this pattern when there are more than 2 args or when the
	// second arg looks like a flag (starts with "-").
	if len(args) > 2 || (len(args) == 2 && strings.HasPrefix(args[1], "-")) {
		return profileCreateDirectFlagError(args[0])
	}

	name := args[0]

	// ── Parse and validate the profile name ──────────────────────────────────

	parsedName, err := profile.ParseName(name)
	if err != nil {
		if strings.HasPrefix(name, "searchit ") || strings.HasPrefix(name, "scan ") || strings.HasPrefix(name, "fuzz ") {
			return fmt.Errorf("the first argument appears to be a command string rather than a profile name.\n\nDid you forget the profile name?\n\nUsage:\n  searchit profile create <profile> %q", name)
		}
		return err
	}
	tool := parsedName.Tool

	if tool != "scan" && tool != "fuzz" {
		return fmt.Errorf("unsupported profile namespace %q (supported: scan, fuzz)", tool)
	}

	// ── Determine creation mode ───────────────────────────────────────────────

	var configNode yaml.Node

	if len(args) == 2 {
		// ── Import mode: second argument is the command string ────────────────
		cmdStr := args[1]

		var factory func() *pflag.FlagSet
		if tool == "scan" {
			factory = func() *pflag.FlagSet {
				cmd, _ := NewScanCmd()
				return cmd.Flags()
			}
		} else if tool == "fuzz" {
			factory = func() *pflag.FlagSet {
				cmd, _ := NewFuzzCmd()
				return cmd.Flags()
			}
		}
		cfgNode, warnings, err := importer.ParseCommand(tool, cmdStr, factory)
		if err != nil {
			return fmt.Errorf("cannot import command: %w", err)
		}

		// Print any informational warnings for runtime-only flags.
		for _, w := range warnings {
			fmt.Fprintf(cmd.ErrOrStderr(), "  note: %s\n", w)
		}

		configNode = cfgNode
	} else {
		// ── Empty-template mode: create an empty config mapping ───────────────
		configNode.Kind = yaml.MappingNode
	}

	// ── Build and store the profile ───────────────────────────────────────────

	p := profile.Profile{
		Schema: 1,
		Name:   name,
		Tool:   tool,
		Config: configNode,
	}

	store := profile.NewStore()
	if err := store.Create(p); err != nil {
		return err
	}

	// ── Success output ────────────────────────────────────────────────────────

	userDir := getUserDir()
	filePath := filepath.Join(userDir, name+".yaml")

	fmt.Fprintln(cmd.OutOrStdout(), "Created profile:", name)
	fmt.Fprintln(cmd.OutOrStdout(), "Path:           ", filePath)

	if len(configNode.Content) > 0 {
		fields := len(configNode.Content) / 2
		fmt.Fprintf(cmd.OutOrStdout(), "Imported:        %d field(s)\n", fields)
	}

	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Review with:")
	fmt.Fprintf(cmd.OutOrStdout(), "  searchit profile show %s\n", name)

	return nil
}

// profileCreateDirectFlagError returns the helpful error shown when the user
// passes scan/fuzz flags directly to "profile create" instead of wrapping
// them in a quoted command string.
func profileCreateDirectFlagError(profileName string) error {
	tool := "scan"
	if parts := strings.SplitN(profileName, "/", 2); len(parts) == 2 &&
		(parts[0] == "fuzz" || parts[0] == "scan") {
		tool = parts[0]
	}

	return fmt.Errorf(`profile creation does not accept %s flags directly.

To create a profile from an existing command, pass it as a quoted string:

    searchit profile create %s \
        "searchit %s -t 128 --adaptive"

Or create an empty profile:

    searchit profile create %s`,
		tool, profileName, tool, profileName)
}

func getUserDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "searchit", "profiles")
}
