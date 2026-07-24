package env

import (
	"os"
	"os/exec"
	"strings"

	"github.com/unsubble/searchit/internal/testutil/command"
)

type InstallContext struct {
	ActiveExecutable    string
	InstallationMethod  string
	InstalledExecutable string
}

type MultipleBinaryResult struct {
	HasMultiple bool
	UniquePaths []string
	Versions    map[string]string
}

func ResolveInstallContext(executor command.Executor) InstallContext {
	if executor == nil {
		executor = command.DefaultExecutor{}
	}

	ctx := InstallContext{}
	exe, err := os.Executable()
	if err == nil {
		ctx.ActiveExecutable = exe
	} else {
		ctx.ActiveExecutable = "UNKNOWN"
	}

	gobin, _ := getGoEnv(executor, "GOBIN")
	if gobin == "" {
		gopath, _ := getGoEnv(executor, "GOPATH")
		if gopath != "" {
			gobin = strings.Split(gopath, string(os.PathListSeparator))[0] + "/bin"
		} else {
			home, _ := os.UserHomeDir()
			if home != "" {
				gobin = home + "/go/bin"
			}
		}
	}

	if gobin != "" {
		ctx.InstalledExecutable = gobin + "/searchit"
	} else {
		ctx.InstalledExecutable = "UNKNOWN"
	}

	if ctx.ActiveExecutable != "UNKNOWN" && ctx.InstalledExecutable != "UNKNOWN" {
		if ctx.ActiveExecutable == ctx.InstalledExecutable {
			ctx.InstallationMethod = "GO INSTALLATION"
		} else {
			if strings.HasPrefix(ctx.ActiveExecutable, "/usr/") || strings.HasPrefix(ctx.ActiveExecutable, "/opt/") || strings.HasPrefix(ctx.ActiveExecutable, "/usr/local/") {
				ctx.InstallationMethod = "SYSTEM INSTALLATION"
			} else {
				ctx.InstallationMethod = "UNKNOWN INSTALLATION"
			}
		}
	} else {
		ctx.InstallationMethod = "UNKNOWN INSTALLATION"
	}

	return ctx
}

func getGoEnv(executor command.Executor, key string) (string, error) {
	cmd := executor.Command("go", "env", key)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func CheckMultipleInstallations(executor command.Executor) MultipleBinaryResult {
	if executor == nil {
		executor = command.DefaultExecutor{}
	}

	result := MultipleBinaryResult{
		UniquePaths: make([]string, 0),
		Versions:    make(map[string]string),
	}

	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return result
	}

	dirs := strings.Split(pathEnv, string(os.PathListSeparator))
	seen := make(map[string]bool)

	// Check each directory in PATH
	for _, dir := range dirs {
		if dir == "" {
			continue
		}

		// Optional: handle windows extensions if needed in the future
		// For now we look for the exact binary name "searchit"
		candidate := dir + string(os.PathSeparator) + "searchit"
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			if !seen[candidate] {
				seen[candidate] = true
				result.UniquePaths = append(result.UniquePaths, candidate)
			}
		}
	}

	if len(result.UniquePaths) > 1 {
		result.HasMultiple = true
		for _, p := range result.UniquePaths {
			vOut, _ := exec.Command(p, "--version").Output()
			vStr := strings.TrimSpace(string(vOut))
			if vStr == "" {
				vStr = "unknown version"
			}
			result.Versions[p] = vStr
		}
	}

	return result
}
