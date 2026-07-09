package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/profile"
)

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available profiles",
	Long:  `List all discoverable profiles grouped by source (BUILTIN / USER) and show tags.`,
	RunE:  runProfileList,
}

func init() {
	profileCmd.AddCommand(profileListCmd)
}

func runProfileList(cmd *cobra.Command, args []string) error {
	store := profile.NewStore()
	profiles, err := store.List()
	if err != nil {
		return fmt.Errorf("list profiles: %w", err)
	}

	// Group by builtin vs user.
	var builtins, user []profile.ProfileInfo
	for _, p := range profiles {
		if p.Builtin {
			builtins = append(builtins, p)
		} else {
			user = append(user, p)
		}
	}

	if len(builtins) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "BUILTIN")
		fmt.Fprintln(cmd.OutOrStdout())
		for _, p := range builtins {
			if len(p.Tags) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%s (tags: %s)\n", p.Name, strings.Join(p.Tags, ", "))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), p.Name)
			}
		}
	}

	if len(user) > 0 {
		if len(builtins) > 0 {
			fmt.Fprintln(cmd.OutOrStdout())
		}
		fmt.Fprintln(cmd.OutOrStdout(), "USER")
		fmt.Fprintln(cmd.OutOrStdout())
		for _, p := range user {
			if len(p.Tags) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%s (tags: %s)\n", p.Name, strings.Join(p.Tags, ", "))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), p.Name)
			}
		}
	}

	return nil
}
