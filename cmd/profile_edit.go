package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/editor"
)

var profileEditCmd = &cobra.Command{
	Use:   "edit [profile]",
	Short: "Edit a profile safely",
	Long: `Open a profile in a text editor.
If the profile is valid upon saving, it is saved atomically.
If it is a built-in profile, a copy is saved to the user profiles directory.
If validation fails, the temporary file is kept and changes are not applied.`,
	Args: cobra.ExactArgs(1),
	RunE: runProfileEdit,
}

func init() {
	profileCmd.AddCommand(profileEditCmd)
}

func runProfileEdit(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	store := profile.NewStore()
	session := editor.NewSession(store)

	if err := session.Run(args[0]); err != nil {
		if valErr, ok := err.(*editor.ValidationError); ok {
			fmt.Fprintln(cmd.OutOrStderr(), valErr.Error())
			return fmt.Errorf("validation failed")
		}
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Successfully edited profile: %s\n", args[0])
	return nil
}
