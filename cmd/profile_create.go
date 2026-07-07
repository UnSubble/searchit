package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/profile"
	"gopkg.in/yaml.v3"
)

var profileCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new profile",
	Long: `Create an empty profile skeleton.

The namespace determines the tool (e.g., scan/myprofile -> tool is scan).`,
	Args: cobra.ExactArgs(1),
	RunE: runProfileCreate,
}

func init() {
	profileCmd.AddCommand(profileCreateCmd)
}

func runProfileCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	// 1. Parse and validate name
	parsedName, err := profile.ParseName(name)
	if err != nil {
		return err
	}
	tool := parsedName.Tool

	// 2. Initialize empty config mapping node: {}
	var configNode yaml.Node
	configNode.Kind = yaml.MappingNode

	p := profile.Profile{
		Version:     1,
		Name:        name,
		Tool:        tool,
		Description: "",
		Config:      configNode,
	}

	store := profile.NewStore()
	if err := store.Create(p); err != nil {
		return err
	}

	// 3. Resolve path to print in the message
	userDir := getUserDir()
	filePath := filepath.Join(userDir, name+".yaml")

	fmt.Fprintln(cmd.OutOrStdout(), "Created profile:")
	fmt.Fprintln(cmd.OutOrStdout(), name)
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Path:")
	fmt.Fprintln(cmd.OutOrStdout(), filePath)

	return nil
}

func getUserDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "searchit", "profiles")
}
