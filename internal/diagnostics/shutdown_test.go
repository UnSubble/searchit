package diagnostics_test

import (
	"context"
	"testing"
	"time"

	"github.com/unsubble/searchit/internal/diagnostics"
)

func TestRunDiagnostics_QuickExitInTests(t *testing.T) {
	// In test binaries, RunDiagnostics returns immediately.
	start := time.Now()
	diagnostics.RunDiagnostics(5*time.Second, context.Canceled, context.Canceled)
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("RunDiagnostics took too long (%v), expected quick exit in tests", elapsed)
	}
}
