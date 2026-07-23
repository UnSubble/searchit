package release

import (
	"fmt"
	"strings"

	"github.com/unsubble/searchit/internal/semver"
	"github.com/unsubble/searchit/internal/testutil/command"
)

type Manager struct {
	Executor command.Executor
}

func NewManager() *Manager {
	return &Manager{
		Executor: command.DefaultExecutor{},
	}
}

type CommitAnalysis struct {
	TotalCommits int
	NewCommands  bool
	Breaking     bool
	Added        int
	Fixed        int
	Refactored   int
	Optimized    int
	Security     int
	Removed      int
	Docs         int
}

// AnalyzeCommits parses the git log from the last tag to HEAD.
func (m *Manager) AnalyzeCommits(lastTag string) (CommitAnalysis, error) {
	rangeStr := "HEAD"
	if lastTag != "" {
		rangeStr = fmt.Sprintf("%s..HEAD", lastTag)
	}

	cmd := m.Executor.Command("git", "log", "--oneline", rangeStr)
	out, err := cmd.Output()
	if err != nil {
		return CommitAnalysis{}, fmt.Errorf("failed to git log %s: %w", rangeStr, err)
	}

	analysis := CommitAnalysis{}
	lines := strings.Split(string(out), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		analysis.TotalCommits++

		upperLine := strings.ToUpper(line)
		if strings.Contains(upperLine, "[ADD]") {
			analysis.Added++
			if strings.Contains(upperLine, "COMMAND") || strings.Contains(upperLine, "CMD:") {
				analysis.NewCommands = true
			}
		}
		if strings.Contains(upperLine, "[FIX]") {
			analysis.Fixed++
		}
		if strings.Contains(upperLine, "[REFINE]") || strings.Contains(upperLine, "[REFACTOR]") {
			analysis.Refactored++
		}
		if strings.Contains(upperLine, "[OPTIMIZE]") {
			analysis.Optimized++
		}
		if strings.Contains(upperLine, "[BREAKING]") {
			analysis.Breaking = true
		}
		if strings.Contains(upperLine, "[SECURITY]") {
			analysis.Security++
		}
		if strings.Contains(upperLine, "[REMOVE]") {
			analysis.Removed++
		}
	}

	return analysis, nil
}

// ValidateBump evaluates whether a requested version bump is scientifically valid
// based on the commit analysis.
func ValidateBump(bumpType string, analysis CommitAnalysis) string {
	switch bumpType {
	case "patch":
		if analysis.Breaking {
			return "PATCH RELEASE MAY BE INVALID\n        REASON:\n            [BREAKING] COMMIT DETECTED\n        SUGGESTION:\n            MAJOR RELEASE IS RECOMMENDED"
		}
		if analysis.NewCommands || analysis.Added > 0 {
			return "PATCH RELEASE MAY BE INVALID\n        REASON:\n            NEW FEATURES OR COMMANDS DETECTED\n        SUGGESTION:\n            MINOR RELEASE IS RECOMMENDED"
		}
		return "PATCH RELEASE VALID"
	case "minor":
		if analysis.Breaking {
			return "MINOR RELEASE MAY BE INVALID\n        REASON:\n            [BREAKING] COMMIT DETECTED\n        SUGGESTION:\n            MAJOR RELEASE IS RECOMMENDED"
		}
		return "MINOR RELEASE VALID"
	case "major":
		if !analysis.Breaking && analysis.Added == 0 && analysis.Fixed == 0 {
			return "MAJOR RELEASE POSSIBLY INCORRECT\n        REASON:\n            NO MEANINGFUL CHANGES DETECTED\n        SUGGESTION:\n            PATCH RELEASE IS RECOMMENDED"
		}
		return "MAJOR RELEASE VALID"
	}
	return "UNKNOWN BUMP TYPE"
}

// GetLastTag scientifically finds the latest tag using git.
func (m *Manager) GetLastTag() string {
	cmd := m.Executor.Command("git", "describe", "--tags", "--abbrev=0")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GenerateNewsPreview interacts with the NEWS system to produce the markdown preview.
func GenerateNewsPreview(target semver.Version, analysis CommitAnalysis) string {
	var builder strings.Builder

	builder.WriteString("--------------------------------------------------\n\n")
	builder.WriteString("                NEWS PREVIEW\n\n")
	builder.WriteString("--------------------------------------------------\n\n\n")

	builder.WriteString("VERSION\n\n")
	builder.WriteString(fmt.Sprintf("        %s\n\n\n", target.Original))

	builder.WriteString("TITLE\n\n")
	builder.WriteString(fmt.Sprintf("        RELEASE %s\n\n\n", target.Original))

	if analysis.Added > 0 {
		builder.WriteString("--------------------------------------------------\n\n\n")
		builder.WriteString("NEW\n\n")
		builder.WriteString(fmt.Sprintf("        - Added %d new features.\n\n\n", analysis.Added))
	}

	if analysis.Refactored > 0 || analysis.Optimized > 0 {
		builder.WriteString("--------------------------------------------------\n\n\n")
		builder.WriteString("IMPROVED\n\n")
		builder.WriteString(fmt.Sprintf("        - Improved codebase (%d refactors, %d optimizations).\n\n\n", analysis.Refactored, analysis.Optimized))
	}

	if analysis.Fixed > 0 {
		builder.WriteString("--------------------------------------------------\n\n\n")
		builder.WriteString("FIXED\n\n")
		builder.WriteString(fmt.Sprintf("        - Fixed %d issues.\n\n\n", analysis.Fixed))
	}

	return builder.String()
}
