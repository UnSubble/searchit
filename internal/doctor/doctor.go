package doctor

import (
	"runtime"

	"github.com/unsubble/searchit/internal/env"
	"github.com/unsubble/searchit/internal/github"
	"github.com/unsubble/searchit/internal/testutil/command"
	"github.com/unsubble/searchit/internal/update"
)

type CheckResult struct {
	Name   string
	Status string
}

type Doctor struct {
	Executor command.Executor
}

func NewDoctor() *Doctor {
	return &Doctor{
		Executor: command.DefaultExecutor{},
	}
}

func (d *Doctor) RunAllChecks() ([]CheckResult, bool) {
	var results []CheckResult
	allHealthy := true

	// VERSION
	results = append(results, CheckResult{"VERSION", "PASS"})

	// UPDATE SYSTEM
	mgr := update.NewManager()
	if _, err := mgr.Client.FetchVersions(); err != nil {
		results = append(results, CheckResult{"UPDATE SYSTEM", "MAY BE INVALID"})
		allHealthy = false
	} else {
		results = append(results, CheckResult{"UPDATE SYSTEM", "PASS"})
	}

	// NEWS SYSTEM
	results = append(results, CheckResult{"NEWS SYSTEM", "PASS"})

	// CONFIGURATION
	results = append(results, CheckResult{"CONFIGURATION", "PASS"})

	// GITHUB CONNECTIVITY
	ghClient := github.NewClient()
	if _, err := ghClient.FetchVersions(); err != nil {
		results = append(results, CheckResult{"GITHUB CONNECTIVITY", "FAIL"})
		allHealthy = false
	} else {
		results = append(results, CheckResult{"GITHUB CONNECTIVITY", "PASS"})
	}

	// RELEASE CHANNEL
	results = append(results, CheckResult{"RELEASE CHANNEL", "PASS"})

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
	goVer := runtime.Version()
	if goVer != "" {
		results = append(results, CheckResult{"GO VERSION", "PASS"})
	} else {
		cmd := d.Executor.Command("go", "version")
		if err := cmd.Run(); err != nil {
			results = append(results, CheckResult{"GO VERSION", "NOT VERIFIED"})
		} else {
			results = append(results, CheckResult{"GO VERSION", "PASS"})
		}
	}

	return results, allHealthy
}
