package doctor

import (
	"github.com/unsubble/searchit/internal/env"
	"github.com/unsubble/searchit/internal/github"
	"github.com/unsubble/searchit/internal/news"
	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/semver"
	"github.com/unsubble/searchit/internal/testutil/command"
	"github.com/unsubble/searchit/internal/update"
	"github.com/unsubble/searchit/internal/version"
)

type CheckResult struct {
	Name   string
	Status string
}

type Doctor struct {
	Executor     command.Executor
	ProfileStore profile.Store
}

func NewDoctor() *Doctor {
	return &Doctor{
		Executor:     command.DefaultExecutor{},
		ProfileStore: profile.NewStore(),
	}
}

func (d *Doctor) RunAllChecks() ([]CheckResult, bool) {
	var results []CheckResult
	allHealthy := true

	// VERSION
	if _, err := semver.Parse(version.Version); err != nil {
		results = append(results, CheckResult{"VERSION", "FAIL"})
		allHealthy = false
	} else {
		results = append(results, CheckResult{"VERSION", "PASS"})
	}

	// UPDATE SYSTEM
	mgr := update.NewManager()
	if _, err := mgr.Client.FetchVersions(); err != nil {
		results = append(results, CheckResult{"UPDATE SYSTEM", "MAY BE INVALID"})
		allHealthy = false
	} else {
		results = append(results, CheckResult{"UPDATE SYSTEM", "PASS"})
	}

	// NEWS SYSTEM
	newsDir := news.GetNewsDir()
	if newsDir == "" {
		results = append(results, CheckResult{"NEWS SYSTEM", "FAIL"})
		allHealthy = false
	} else {
		results = append(results, CheckResult{"NEWS SYSTEM", "PASS"})
	}

	// CONFIGURATION
	_ = profile.RegisterBuiltinDecoders()
	store := d.ProfileStore
	if store == nil {
		store = profile.NewStore()
	}
	if _, err := store.List(); err != nil {
		results = append(results, CheckResult{"CONFIGURATION", "FAIL"})
		allHealthy = false
	} else {
		results = append(results, CheckResult{"CONFIGURATION", "PASS"})
	}

	// GITHUB CONNECTIVITY
	ghClient := github.NewClient()
	if _, err := ghClient.FetchVersions(); err != nil {
		results = append(results, CheckResult{"GITHUB CONNECTIVITY", "FAIL"})
		allHealthy = false
	} else {
		results = append(results, CheckResult{"GITHUB CONNECTIVITY", "PASS"})
	}

	// RELEASE CHANNEL
	if v, err := semver.Parse(version.Version); err != nil || (v.Channel() != "stable" && v.Channel() != "experimental") {
		results = append(results, CheckResult{"RELEASE CHANNEL", "FAIL"})
		allHealthy = false
	} else {
		results = append(results, CheckResult{"RELEASE CHANNEL", "PASS"})
	}

	// MULTIPLE BINARIES
	mult := env.CheckMultipleInstallations(d.Executor)
	if mult.HasMultiple {
		results = append(results, CheckResult{"MULTIPLE BINARIES", "WARNING"})
		allHealthy = false
	} else {
		results = append(results, CheckResult{"MULTIPLE BINARIES", "PASS"})
	}

	// INSTALLATION / ACTIVE EXECUTABLE
	ctx := env.ResolveInstallContext(d.Executor)
	if ctx.ActiveExecutable != "UNKNOWN" {
		results = append(results, CheckResult{"ACTIVE EXECUTABLE", "PASS"})
	} else {
		results = append(results, CheckResult{"ACTIVE EXECUTABLE", "WARNING"})
		allHealthy = false
	}

	if ctx.InstallationMethod == "GO INSTALLATION" {
		results = append(results, CheckResult{"INSTALLATION METHOD", "PASS"})
	} else {
		results = append(results, CheckResult{"INSTALLATION METHOD", "WARNING"})
		allHealthy = false
	}

	// RECOMMENDATION
	if mult.HasMultiple {
		results = append(results, CheckResult{"RECOMMENDATION", "Multiple Searchit installations were detected. Consider removing unused executables or adjusting your PATH."})
	}

	// GO VERSION
	cmd := d.Executor.Command("go", "version")
	if err := cmd.Run(); err != nil {
		results = append(results, CheckResult{"GO VERSION", "NOT VERIFIED"})
		allHealthy = false
	} else {
		results = append(results, CheckResult{"GO VERSION", "PASS"})
	}

	return results, allHealthy
}
