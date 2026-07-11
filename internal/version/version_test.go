package version_test

import (
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/version"
)

func TestVersion_String(t *testing.T) {
	s := version.String()
	if !strings.HasPrefix(s, "searchit v") {
		t.Errorf("String() returned %q, expected prefix 'searchit v'", s)
	}
}

func TestVersion_Long(t *testing.T) {
	l := version.Long()
	if !strings.HasPrefix(l, "searchit v") {
		t.Errorf("Long() returned %q, expected prefix 'searchit v'", l)
	}
	if !strings.Contains(l, "Commit:") {
		t.Errorf("Long() output missing 'Commit:', got:\n%s", l)
	}
	if !strings.Contains(l, "Built:") {
		t.Errorf("Long() output missing 'Built:', got:\n%s", l)
	}
}
