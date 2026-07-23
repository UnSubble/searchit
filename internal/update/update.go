package update

import (
	"fmt"
	"os"
	"strings"

	"github.com/unsubble/searchit/internal/github"
	"github.com/unsubble/searchit/internal/news"
	"github.com/unsubble/searchit/internal/semver"
	"github.com/unsubble/searchit/internal/testutil/command"
	"github.com/unsubble/searchit/internal/version"
)

type Manager struct {
	Client   *github.Client
	Executor command.Executor
}

func NewManager() *Manager {
	return &Manager{
		Client:   github.NewClient(),
		Executor: command.DefaultExecutor{},
	}
}

type CheckResult struct {
	CurrentVersion semver.Version
	TargetVersion  semver.Version
	LatestStable   semver.Version
	LatestAny      semver.Version
	Status         string
	IsUpdate       bool
	IsRollback     bool
	IsDowngrade    bool
	IsValidTarget  bool
}

// Check evaluates the ecosystem status for the current binary.
func (m *Manager) Check(experimental bool, targetVersionStr string, isRollback bool) (CheckResult, error) {
	current, err := semver.Parse(version.Version)
	if err != nil {
		// Fallback for dev builds
		current = semver.Version{Original: "dev", Major: 0, Minor: 0, Patch: 0}
	}

	stable, err := m.Client.GetLatestStable()
	var latestAny semver.Version
	if err == nil {
		latestAny = stable
	}
	if exp, err := m.Client.GetLatest(); err == nil {
		if stable.Original == "" || exp.Compare(stable) > 0 {
			latestAny = exp
		}
	}

	var target semver.Version
	if targetVersionStr != "" {
		if targetVersionStr == "latest-stable" {
			target = stable
		} else {
			target, err = semver.Parse(targetVersionStr)
			if err != nil {
				return CheckResult{}, fmt.Errorf("invalid target version: %w", err)
			}
		}
	} else {
		if experimental {
			target = latestAny
		} else {
			target = stable
		}
	}

	result := CheckResult{
		CurrentVersion: current,
		TargetVersion:  target,
		LatestStable:   stable,
		LatestAny:      latestAny,
		IsValidTarget:  target.Original != "",
	}

	if result.IsValidTarget {
		cmp := target.Compare(current)
		if cmp > 0 {
			result.IsUpdate = true
			if !isRollback {
				result.Status = "UPDATE AVAILABLE"
			} else {
				result.Status = "ROLLBACK TARGET IS NEWER (POSSIBLY INCORRECT)"
			}
		} else if cmp < 0 {
			result.IsDowngrade = true
			if isRollback {
				result.IsRollback = true
				result.Status = "ROLLBACK AVAILABLE"
			} else {
				result.Status = "DOWNGRADE REQUESTED (WARNING)"
			}
		} else {
			result.Status = "UP TO DATE"
		}
	} else {
		result.Status = "UNKNOWN"
	}

	return result, nil
}

// PreviewAction dry-runs an update/rollback, reporting but not modifying.
func (m *Manager) PreviewAction(res CheckResult, action string) {
	fmt.Printf("        CURRENT VERSION\n\n                %s\n\n\n", res.CurrentVersion.Original)
	fmt.Printf("        TARGET VERSION\n\n                %s\n\n\n", res.TargetVersion.Original)
	fmt.Printf("        RELEASE CHANNEL\n\n                %s\n\n\n", res.TargetVersion.Channel())

	fmt.Println("        NEWS")
	n, err := news.Fetch(res.TargetVersion.Original)
	if err != nil {
		fmt.Printf("                NOT VERIFIED (%s)\n\n\n", err.Error())
	} else {
		lines := strings.Split(n.Content, "\n")
		for _, l := range lines {
			fmt.Printf("                %s\n", l)
		}
		fmt.Printf("\n\n")
	}

	actionText := strings.ToUpper(action)
	if res.IsDowngrade && action != "rollback" {
		actionText = "DOWNGRADE"
	}
	fmt.Printf("        ACTION SUMMARY\n\n                %s to %s\n\n\n", actionText, res.TargetVersion.Original)

	if action == "rollback" {
		fmt.Println("        READY TO ROLLBACK")
	} else {
		fmt.Println("        READY TO INSTALL")
	}
}

// Execute installs the target version.
func (m *Manager) Execute(target semver.Version) error {
	pkg := fmt.Sprintf("github.com/unsubble/searchit@%s", target.Original)

	fmt.Printf("Executing: go install %s\n", pkg)

	cmd := m.Executor.Command("go", "install", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
