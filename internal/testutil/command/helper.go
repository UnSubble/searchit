package command

import (
	"fmt"
	"os"
)

// HandleHelperProcess must be called by a TestHelperProcess function in the _test.go file
// of the package utilizing MockExecutor. It intercepts the execution and mocks output.
func HandleHelperProcess() {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Print(os.Getenv("MOCK_OUTPUT"))
	var exitCode int
	fmt.Sscanf(os.Getenv("MOCK_EXIT_CODE"), "%d", &exitCode)
	os.Exit(exitCode)
}
