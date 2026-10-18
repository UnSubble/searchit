package doctor

import (
	"fmt"
	"strings"

	"github.com/unsubble/searchit/internal/env"
	"github.com/unsubble/searchit/internal/github"
	"github.com/unsubble/searchit/internal/news"
	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/semver"
	"github.com/unsubble/searchit/internal/testutil/command"
	"github.com/unsubble/searchit/internal/version"
)

// Status constants for CheckResult.Status.
const (
	StatusPass        = "PASS"
	StatusWarn        = "WARN"
	StatusFail        = "FAIL"
	StatusNotVerified = "NOT VERIFIED"
)

// CheckResult holds the outcome of a single health check.
type CheckResult struct {
	Name    string // display name of the check
	Status  string // one of StatusPass, StatusWarn, StatusFail, StatusNotVerified
	Message string // human-readable diagnostic detail; always set
}

// Summary aggregates all check results.
type Summary struct {
	Results       []CheckResult
	HasFail       bool // at least one check is StatusFail
	HasWarn       bool // at least one check is StatusWarn
	HasUnverified bool // at least one check is StatusNotVerified
}

// Doctor runs ecosystem health checks.
type Doctor struct {
	Executor     command.Executor
	ProfileStore profile.Store
	GitHubClient *github.Client
	// NewsVersion is the version string used when probing the NEWS system.
	// Defaults to version.Version if empty.
	NewsVersion string
}

// NewDoctor creates a Doctor with real, production-grade dependencies.
func NewDoctor() *Doctor {
	return &Doctor{
		Executor:     command.DefaultExecutor{},
		ProfileStore: profile.NewStore(),
		GitHubClient: github.NewClient(),
	}
}

// RunAllChecks executes all health checks sequentially and returns a Summary.
func (d *Doctor) RunAllChecks() Summary {
	s := Summary{}

	add := func(r CheckResult) {
		s.Results = append(s.Results, r)
		switch r.Status {
		case StatusFail:
			s.HasFail = true
		case StatusWarn:
			s.HasWarn = true
		case StatusNotVerified:
			s.HasUnverified = true
		}
	}

	// 1. VERSION
	add(d.checkVersion())

	// 2. GO VERSION
	add(d.checkGoVersion())

	// 3. CONFIGURATION
	add(d.checkConfiguration())

	// 4. NEWS SYSTEM
	add(d.checkNewsSystem())

	// 5. GITHUB CONNECTIVITY
	ghResult, versions := d.checkGitHubConnectivity()
	add(ghResult)

	// 6. UPDATE SYSTEM (reuses already-fetched versions to avoid a second network call)
	add(d.checkUpdateSystem(versions))

	// 7. MULTIPLE BINARIES
	add(d.checkMultipleBinaries())

	// 8. ACTIVE EXECUTABLE
	add(d.checkActiveExecutable())

	return s
}

func (d *Doctor) checkVersion() CheckResult {
	v, err := semver.Parse(version.Version)
	if err != nil {
		return CheckResult{
			Name:    "VERSION",
			Status:  StatusFail,
			Message: fmt.Sprintf("invalid version %q: %v", version.Version, err),
		}
	}
	return CheckResult{
		Name:    "VERSION",
		Status:  StatusPass,
		Message: v.Original,
	}
}

func (d *Doctor) checkGoVersion() CheckResult {
	exec := d.Executor
	if exec == nil {
		exec = command.DefaultExecutor{}
	}
	cmd := exec.Command("go", "version")
	out, err := cmd.Output()
	if err != nil {
		return CheckResult{
			Name:    "GO VERSION",
			Status:  StatusNotVerified,
			Message: "go not found on PATH (not required for pre-built binary users)",
		}
	}
	return CheckResult{
		Name:    "GO VERSION",
		Status:  StatusPass,
		Message: strings.TrimSpace(string(out)),
	}
}

func (d *Doctor) checkConfiguration() CheckResult {
	_ = profile.RegisterBuiltinDecoders()

	store := d.ProfileStore
	if store == nil {
		store = profile.NewStore()
	}

	profiles, err := store.List()
	if err != nil {
		return CheckResult{
			Name:    "CONFIGURATION",
			Status:  StatusFail,
			Message: fmt.Sprintf("profile store error: %v", err),
		}
	}
	return CheckResult{
		Name:    "CONFIGURATION",
		Status:  StatusPass,
		Message: fmt.Sprintf("profile store loaded, %d profile(s) available", len(profiles)),
	}
}

func (d *Doctor) checkNewsSystem() CheckResult {
	newsVersion := d.NewsVersion
	if newsVersion == "" {
		newsVersion = version.Version
	}

	n, err := news.Fetch(newsVersion)
	if err != nil {
		// News not found is a WARNING, not a FAIL — it is optional in deployed environments.
		if strings.Contains(err.Error(), "no news available") {
			return CheckResult{
				Name:    "NEWS SYSTEM",
				Status:  StatusWarn,
				Message: fmt.Sprintf("no news file found for %s", newsVersion),
			}
		}
		// Any other I/O error (e.g., permission denied) is a FAIL.
		return CheckResult{
			Name:    "NEWS SYSTEM",
			Status:  StatusFail,
			Message: fmt.Sprintf("news system error: %v", err),
		}
	}
	return CheckResult{
		Name:    "NEWS SYSTEM",
		Status:  StatusPass,
		Message: fmt.Sprintf("news available for %s", n.Version),
	}
}

// checkGitHubConnectivity returns the check result and the fetched versions slice
// (which may be nil on failure) so the UPDATE SYSTEM check can reuse them.
func (d *Doctor) checkGitHubConnectivity() (CheckResult, []semver.Version) {
	client := d.GitHubClient
	if client == nil {
		client = github.NewClient()
	}

	versions, err := client.FetchVersions()
	if err != nil {
		return CheckResult{
			Name:    "GITHUB CONNECTIVITY",
			Status:  StatusFail,
			Message: fmt.Sprintf("GitHub API request failed: %v", err),
		}, nil
	}
	return CheckResult{
		Name:    "GITHUB CONNECTIVITY",
		Status:  StatusPass,
		Message: fmt.Sprintf("GitHub API reachable, %d release(s) found", len(versions)),
	}, versions
}

func (d *Doctor) checkUpdateSystem(versions []semver.Version) CheckResult {
	if versions == nil {
		// GitHub connectivity already failed — we have no data to work with.
		return CheckResult{
			Name:    "UPDATE SYSTEM",
			Status:  StatusFail,
			Message: "update metadata unavailable (GitHub connectivity failed)",
		}
	}

	current, err := semver.Parse(version.Version)
	if err != nil {
		return CheckResult{
			Name:    "UPDATE SYSTEM",
			Status:  StatusFail,
			Message: fmt.Sprintf("cannot evaluate update status: invalid current version %q", version.Version),
		}
	}

	// Find latest stable from the already-fetched versions.
	var latestStable semver.Version
	for _, v := range versions {
		if v.IsStable() {
			latestStable = v
			break // versions are sorted highest-first
		}
	}

	if latestStable.Original == "" {
		return CheckResult{
			Name:    "UPDATE SYSTEM",
			Status:  StatusPass,
			Message: fmt.Sprintf("current: %s (no stable release found on GitHub)", current.Original),
		}
	}

	cmp := current.Compare(latestStable)
	var msg string
	switch {
	case cmp > 0:
		msg = fmt.Sprintf("current: %s — ahead of latest stable (%s)", current.Original, latestStable.Original)
	case cmp == 0:
		msg = fmt.Sprintf("current: %s — up to date", current.Original)
	default:
		msg = fmt.Sprintf("current: %s — update available: %s", current.Original, latestStable.Original)
	}

	return CheckResult{
		Name:    "UPDATE SYSTEM",
		Status:  StatusPass,
		Message: msg,
	}
}

func (d *Doctor) checkMultipleBinaries() CheckResult {
	exec := d.Executor
	if exec == nil {
		exec = command.DefaultExecutor{}
	}

	mult := env.CheckMultipleInstallations(exec)
	if mult.HasMultiple {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%d searchit executables found on PATH:", len(mult.UniquePaths)))
		for _, p := range mult.UniquePaths {
			sb.WriteString("\n        ")
			sb.WriteString(p)
		}
		return CheckResult{
			Name:    "MULTIPLE BINARIES",
			Status:  StatusWarn,
			Message: sb.String(),
		}
	}
	return CheckResult{
		Name:    "MULTIPLE BINARIES",
		Status:  StatusPass,
		Message: "single searchit executable detected",
	}
}

func (d *Doctor) checkActiveExecutable() CheckResult {
	exec := d.Executor
	if exec == nil {
		exec = command.DefaultExecutor{}
	}

	ctx := env.ResolveInstallContext(exec)
	if ctx.ActiveExecutable == "UNKNOWN" {
		return CheckResult{
			Name:    "ACTIVE EXECUTABLE",
			Status:  StatusWarn,
			Message: "could not determine active executable path",
		}
	}
	return CheckResult{
		Name:    "ACTIVE EXECUTABLE",
		Status:  StatusPass,
		Message: ctx.ActiveExecutable,
	}
}
