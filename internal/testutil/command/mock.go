package command

import (
	"fmt"
	"os"
	"os/exec"
)

// MockExecutor implements Executor for deterministic testing.
type MockExecutor struct {
	MockOutput string
	ExitCode   int
}

// Command creates a command that invokes the test binary as a helper process.
func (m *MockExecutor) Command(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		fmt.Sprintf("MOCK_EXIT_CODE=%d", m.ExitCode),
		fmt.Sprintf("MOCK_OUTPUT=%s", m.MockOutput),
	)
	return cmd
}
