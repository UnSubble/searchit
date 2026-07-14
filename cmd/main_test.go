package cmd

import (
	"os"
	"testing"

	"github.com/unsubble/searchit/internal/profile"
)

func TestMain(m *testing.M) {
	_ = profile.RegisterBuiltinValidators()
	_ = profile.RegisterBuiltinDecoders()
	os.Exit(m.Run())
}
