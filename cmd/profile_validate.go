package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/profile"
	_ "github.com/unsubble/searchit/internal/profile/scan"
)

var profileValidateCmd = &cobra.Command{
	Use:   "validate [name]",
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

	// Load profile (which validates file existence)
	p, err := store.Load(name)
	if err != nil {
		fmt.Fprintln(cmd.OutOrStderr(), "Validation failed:")
		fmt.Fprintln(cmd.OutOrStderr(), err.Error())
		return fmt.Errorf("validation failed")
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
