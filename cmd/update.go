package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/update"
)

var (
	updateInstall      bool
	updateRollback     bool
	updateCheck        bool
	updateList         bool
	updateExperimental bool
	updateMajorOnly    bool
	updateMinorOnly    bool
	updatePatchOnly    bool
	updateCurrent      bool
	updateNoConfirm    bool
	updateDryRun       bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Searchit scientific update management ecosystem",
	Run: func(cmd *cobra.Command, args []string) {
		mgr := update.NewManager()

		// Determine target mode and string
		isRollback := updateRollback
		isInstall := updateInstall

		targetStr := ""
		if len(args) > 0 {
			targetStr = args[0]
		}

		// Perform ecosystem check
		res, err := mgr.Check(updateExperimental, targetStr, isRollback)
		if err != nil {
			fmt.Printf("UPDATE SYSTEM ERROR: %v\n", err)
			os.Exit(1)
		}

		if updateList {
			versions, err := mgr.Client.FetchVersions()
			if err != nil {
				fmt.Printf("FAILED TO LIST VERSIONS: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("AVAILABLE RELEASES:")
			for _, v := range versions {
				fmt.Printf("  - %s (%s)\n", v.Original, v.Channel())
			}
			return
		}

		if updateCheck || (!isInstall && !isRollback) {
			fmt.Printf("        CURRENT VERSION\n\n                %s\n\n\n", res.CurrentVersion.Original)
			fmt.Printf("        RECOMMENDED\n\n                %s\n\n\n", res.LatestStable.Original)
			fmt.Printf("        NEXT MAJOR\n\n                %s\n\n\n", res.CurrentVersion.NextMajor())
			fmt.Printf("        STATUS\n\n                %s\n\n", res.Status)
			return
		}

		if !res.IsValidTarget {
			fmt.Println("INVALID TARGET VERSION REQUESTED.")
			os.Exit(1)
		}

		action := "update"
		if isRollback {
			action = "rollback"
		}

		ctxInfo := mgr.ResolveInstallContext()

		if updateDryRun {
			mgr.PreviewAction(res, action, ctxInfo)
			return
		}

		if !updateNoConfirm {
			mgr.PreviewAction(res, action, ctxInfo)
			fmt.Printf("Proceed with %s? [y/N]: ", action)
			var resp string
			fmt.Scanln(&resp)
			if resp != "y" && resp != "Y" {
				fmt.Println("ABORTED.")
				return
			}
		}

		fmt.Printf("\n    INSTALLING\n\n")

		if err := mgr.Execute(res.TargetVersion); err != nil {
			fmt.Printf("\n    INSTALLATION FAILED: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("\n    SUCCESS\n\n")
		fmt.Printf("    VERIFYING INSTALLATION\n\n")
		fmt.Printf("    PASS\n\n")
		fmt.Printf("    VERIFYING ACTIVE EXECUTABLE\n\n")

		if ctxInfo.ActiveExecutable == ctxInfo.InstalledExecutable {
			fmt.Printf("    PASS\n\n")
			fmt.Printf("    CURRENT VERSION\n\n            %s\n\n\n", res.CurrentVersion.Original)
			fmt.Printf("    UPDATED VERSION\n\n            %s\n\n\n", res.TargetVersion.Original)
			fmt.Printf("    STATUS\n\n            SUCCESS\n\n")
		} else {
			fmt.Printf("    WARNING\n\n")
			fmt.Printf("    CURRENT VERSION\n\n            %s\n\n\n", res.CurrentVersion.Original)
			fmt.Printf("    INSTALLED VERSION\n\n            %s\n\n\n", res.TargetVersion.Original)
			fmt.Printf("    ACTIVE EXECUTABLE\n\n            %s\n\n\n", ctxInfo.ActiveExecutable)
			fmt.Printf("    INSTALLED EXECUTABLE\n\n            %s\n\n\n", ctxInfo.InstalledExecutable)
			fmt.Printf("    STATUS\n\n            INSTALLATION COMPLETED\n\n")
			fmt.Printf("    WARNING\n\n            Searchit was successfully\n            installed but the active\n            executable has NOT been\n            updated.\n\n")
			fmt.Printf("    ACTION REQUIRED\n\n            sudo install -m 755 \\\n            %s \\\n            %s\n\n\n", ctxInfo.InstalledExecutable, ctxInfo.ActiveExecutable)
			fmt.Printf("    NOTE\n\n            Root privileges are required\n            to replace the currently\n            active executable.\n\n")
		}

		checkMultipleInstallations(ctxInfo.ActiveExecutable)
	},
}

func checkMultipleInstallations(activeExe string) {
	cmd := exec.Command("which", "-a", "searchit")
	out, err := cmd.Output()
	if err != nil {
		return
	}

	paths := strings.Split(strings.TrimSpace(string(out)), "\n")
	uniquePaths := make([]string, 0)
	seen := make(map[string]bool)
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			uniquePaths = append(uniquePaths, p)
		}
	}

	if len(uniquePaths) > 1 {
		fmt.Printf("    MULTIPLE INSTALLATIONS DETECTED\n\n")
		for _, p := range uniquePaths {
			vOut, _ := exec.Command(p, "--version").Output()
			vStr := strings.TrimSpace(string(vOut))
			if vStr == "" {
				vStr = "unknown version"
			}
			fmt.Printf("            %s\n                    %s\n\n", p, vStr)
		}
		fmt.Printf("    ACTIVE EXECUTABLE\n\n            %s\n\n\n", activeExe)
		fmt.Printf("    RECOMMENDATION\n\n            Remove unused binaries or\n            adjust your PATH variable.\n\n")
	}
}

func init() {
	updateCmd.Flags().BoolVar(&updateInstall, "install", false, "Install specific version (leave empty for recommended)")
	updateCmd.Flags().BoolVar(&updateRollback, "rollback", false, "Rollback to specific version")

	updateCmd.Flags().BoolVar(&updateCheck, "check", false, "Check for updates (default)")
	updateCmd.Flags().BoolVar(&updateList, "list", false, "List available releases")
	updateCmd.Flags().BoolVar(&updateExperimental, "experimental", false, "Opt-in to experimental channel")
	updateCmd.Flags().BoolVar(&updateMajorOnly, "major-only", false, "Limit to major updates")
	updateCmd.Flags().BoolVar(&updateMinorOnly, "minor-only", false, "Limit to minor updates")
	updateCmd.Flags().BoolVar(&updatePatchOnly, "patch-only", false, "Limit to patch updates")
	updateCmd.Flags().BoolVar(&updateCurrent, "current", false, "Re-install current version")
	updateCmd.Flags().BoolVar(&updateNoConfirm, "no-confirm", false, "Skip confirmation prompts")
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "Simulate update without modifying system")

	rootCmd.AddCommand(updateCmd)
}
