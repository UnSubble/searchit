package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/profile"
)

var profileShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show profile details",
	Long:  `Display the full YAML contents of a profile.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileShow,
}

func init() {
	profileCmd.AddCommand(profileShowCmd)
}

func runProfileShow(cmd *cobra.Command, args []string) error {
	name := args[0]
	store := profile.NewStore()

	raw, err := store.LoadRaw(name)
	if err != nil {
		return fmt.Errorf("show profile: %w", err)
	}

	fmt.Fprint(cmd.OutOrStdout(), string(raw))
	return nil
}
