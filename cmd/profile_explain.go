package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/explain"
)

var profileExplainCmd = &cobra.Command{
	Use:   "explain [name]",
	Short: "Explain profile configuration, overrides, and final configuration",
	Long:  `Display target metadata, full dependency chain, override source hierarchy, and the final merged configuration.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileExplain,
}

func init() {
	profileCmd.AddCommand(profileExplainCmd)
}

func runProfileExplain(cmd *cobra.Command, args []string) error {
	name := args[0]
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	store := profile.NewStore()

	model, err := explain.Build(store, name)
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), explain.RenderText(model))
	return nil
}
