package command

import "os/exec"

// Executor defines an interface for executing commands.
type Executor interface {
	Command(name string, arg ...string) *exec.Cmd
}

// DefaultExecutor provides the real implementation using os/exec.
type DefaultExecutor struct{}

// Command creates an *exec.Cmd using the standard os/exec package.
func (DefaultExecutor) Command(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}
