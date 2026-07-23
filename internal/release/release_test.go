package release

import (
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/semver"
	"github.com/unsubble/searchit/internal/testutil/command"
)

func TestHelperProcess(t *testing.T) {
	command.HandleHelperProcess()
}

func TestValidateBump(t *testing.T) {
	tests := []struct {
		name     string
		bumpType string
		analysis CommitAnalysis
		contains []string
	}{
		{
			name:     "patch_valid",
			bumpType: "patch",
			analysis: CommitAnalysis{Fixed: 5},
			contains: []string{"PATCH RELEASE VALID"},
		},
		{
			name:     "patch_invalid_breaking",
			bumpType: "patch",
			analysis: CommitAnalysis{Fixed: 5, Breaking: true},
			contains: []string{"PATCH RELEASE MAY BE INVALID", "[BREAKING] COMMIT DETECTED", "MAJOR RELEASE IS RECOMMENDED"},
		},
		{
			name:     "patch_invalid_new_feature",
			bumpType: "patch",
			analysis: CommitAnalysis{Fixed: 5, Added: 2},
			contains: []string{"PATCH RELEASE MAY BE INVALID", "NEW FEATURES OR COMMANDS DETECTED", "MINOR RELEASE IS RECOMMENDED"},
		},
		{
			name:     "minor_valid",
			bumpType: "minor",
			analysis: CommitAnalysis{Fixed: 5, Added: 2},
			contains: []string{"MINOR RELEASE VALID"},
		},
		{
			name:     "minor_invalid_breaking",
			bumpType: "minor",
			analysis: CommitAnalysis{Added: 5, Breaking: true},
			contains: []string{"MINOR RELEASE MAY BE INVALID", "[BREAKING] COMMIT DETECTED", "MAJOR RELEASE IS RECOMMENDED"},
		},
		{
			name:     "major_valid",
			bumpType: "major",
			analysis: CommitAnalysis{Breaking: true},
			contains: []string{"MAJOR RELEASE VALID"},
		},
		{
			name:     "major_possibly_incorrect_no_changes",
			bumpType: "major",
			analysis: CommitAnalysis{Refactored: 2}, // no added/fixed/breaking
			contains: []string{"MAJOR RELEASE POSSIBLY INCORRECT", "NO MEANINGFUL CHANGES DETECTED", "PATCH RELEASE IS RECOMMENDED"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateBump(tt.bumpType, tt.analysis)
			for _, c := range tt.contains {
				if !strings.Contains(got, c) {
					t.Errorf("ValidateBump() missing %q in %q", c, got)
				}
			}
		})
	}
}

func TestGetLastTag(t *testing.T) {
	mgr := &Manager{
		Executor: &command.MockExecutor{
			MockOutput: "v1.5.0\n",
			ExitCode:   0,
		},
	}

	if got := mgr.GetLastTag(); got != "v1.5.0" {
		t.Errorf("expected v1.5.0, got %q", got)
	}
}

func TestAnalyzeCommits(t *testing.T) {
	mockLog := `[ADD] new feature
[FIX] bug resolved
[BREAKING] api change
[REFACTOR] clean up
[OPTIMIZE] speed up
[SECURITY] patch vulnerability
[REMOVE] old code
`
	mgr := &Manager{
		Executor: &command.MockExecutor{
			MockOutput: mockLog,
			ExitCode:   0,
		},
	}

	analysis, err := mgr.AnalyzeCommits("v1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	if analysis.Added != 1 || analysis.Fixed != 1 || !analysis.Breaking || analysis.Refactored != 1 || analysis.Optimized != 1 || analysis.Security != 1 || analysis.Removed != 1 {
		t.Errorf("analysis parsed incorrectly: %+v", analysis)
	}
}

func TestGenerateNewsPreview(t *testing.T) {
	v, _ := semver.Parse("v1.5.0")
	analysis := CommitAnalysis{
		Added:      3,
		Fixed:      10,
		Refactored: 2,
		Optimized:  5,
	}

	preview := GenerateNewsPreview(v, analysis)

	expectedStrings := []string{
		"VERSION",
		"v1.5.0",
		"NEW",
		"Added 3 new features",
		"IMPROVED",
		"2 refactors, 5 optimizations",
		"FIXED",
		"Fixed 10 issues",
	}

	for _, s := range expectedStrings {
		if !strings.Contains(preview, s) {
			t.Errorf("preview missing expected string: %s", s)
		}
	}
}
