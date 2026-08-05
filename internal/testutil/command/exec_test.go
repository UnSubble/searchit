package command_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/testutil/command"
)

func TestDefaultExecutor(t *testing.T) {
	var execer command.Executor = command.DefaultExecutor{}
	cmd := execer.Command("echo", "hello")
	if cmd == nil || cmd.Path == "" && cmd.Args[0] != "echo" {
		t.Errorf("unexpected cmd from DefaultExecutor: %v", cmd)
	}
}

func TestMockExecutor(t *testing.T) {
	mock := &command.MockExecutor{
		MockOutput: "test output\n",
		ExitCode:   0,
	}

	cmd := mock.Command("arbitrary", "args")
	if cmd == nil {
		t.Fatal("expected non-nil cmd from MockExecutor")
	}
}
