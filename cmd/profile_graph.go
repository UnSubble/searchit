package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/resolver"
)

var profileGraphCmd = &cobra.Command{
	Use:   "graph [name]",
	Short: "Visualize profile dependency tree",
	Long:  `Display a Unicode ASCII tree representation of a profile's dependencies.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileGraph,
}

func init() {
	profileCmd.AddCommand(profileGraphCmd)
}

func runProfileGraph(cmd *cobra.Command, args []string) error {
	name := args[0]
	store := profile.NewStore()

	res := resolver.New(store)
	node, err := res.BuildTree(name)
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), resolver.RenderTree(node))
	return nil
}
