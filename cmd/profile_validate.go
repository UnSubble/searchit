package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/profile"
)

var profileValidateCmd = &cobra.Command{
	Use:   "validate [name|path]",
	Short: "Validate a profile",
	Long: `Perform a two-stage validation on a profile.

First, generic profile structure is validated.
Then, tool-specific configuration is validated.`,
	Args: cobra.ExactArgs(1),
	RunE: runProfileValidate,
}

func init() {
	profileCmd.AddCommand(profileValidateCmd)
}

func runProfileValidate(cmd *cobra.Command, args []string) error {
	name := args[0]
	cmd.SilenceUsage = true

	store := profile.NewStore()

	var p *profile.Profile
	var err error

	// Check if the argument is a local file path
	if info, statErr := os.Stat(name); statErr == nil && !info.IsDir() {
		data, readErr := os.ReadFile(name)
		if readErr != nil {
			fmt.Fprintln(cmd.OutOrStderr(), "Validation failed:")
			fmt.Fprintln(cmd.OutOrStderr(), readErr.Error())
			return fmt.Errorf("validation failed")
		}
		p, err = profile.DecodeProfile(data)
		if err != nil {
			fmt.Fprintln(cmd.OutOrStderr(), "Validation failed:")
			fmt.Fprintln(cmd.OutOrStderr(), err.Error())
			return fmt.Errorf("validation failed")
		}
	} else {
		// Load profile (which validates file existence)
		p, err = store.Load(name)
		if err != nil {
			fmt.Fprintln(cmd.OutOrStderr(), "Validation failed:")
			fmt.Fprintln(cmd.OutOrStderr(), err.Error())
			return fmt.Errorf("validation failed")
		}
	}

	// 1. Generic validation
	if err := profile.Validate(p); err != nil {
		fmt.Fprintln(cmd.OutOrStderr(), "Validation failed:")
		fmt.Fprintln(cmd.OutOrStderr(), err.Error())
		return fmt.Errorf("validation failed")
	}

	// 2. Dispatch to tool-specific validator
	v := profile.GetValidator(p.Tool)
	if v != nil {
		if err := v.Validate(p); err != nil {
			fmt.Fprintln(cmd.OutOrStderr(), "Validation failed:")
			fmt.Fprintln(cmd.OutOrStderr(), err.Error())
			return fmt.Errorf("validation failed")
		}
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Valid profile:")
	fmt.Fprintln(cmd.OutOrStdout(), p.Name)
	return nil
}
