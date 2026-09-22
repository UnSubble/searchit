package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/unsubble/searchit/internal/version"
)

func TestVersionCmd_Output(t *testing.T) {
	rootCmd.SetContext(context.Background())
	versionCmd.SetContext(context.Background())

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	versionCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.SetArgs([]string{"version"})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	ctx := context.Background()
	err := cmd.ExecuteContext(ctx)
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "searchit v") {
		t.Errorf("expected version output to contain 'searchit v', got %q", out)
	}
	if !strings.Contains(out, "Commit:") {
		t.Errorf("expected version output to contain 'Commit:', got %q", out)
	}
	if !strings.Contains(out, "Built:") {
		t.Errorf("expected version output to contain 'Built:', got %q", out)
	}
}

func TestVersionFlag_Output(t *testing.T) {
	rootCmd.SetContext(context.Background())

	cmd := rootCmd
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.SetArgs([]string{"--version"})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	ctx := context.Background()
	err := cmd.ExecuteContext(ctx)
	if err != nil {
		t.Fatalf("version flag failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "searchit v") {
		t.Errorf("expected version flag output to contain 'searchit v', got %q", out)
	}
	if strings.Contains(out, "Commit:") {
		t.Errorf("expected short version flag output to NOT contain 'Commit:', got %q", out)
	}
}

func TestVersionCmd_DevPrereleaseNotDowngrade(t *testing.T) {
	releasesJSON := `[
		{"tag_name": "v0.6.1", "draft": false}
	]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, releasesJSON)
	}))
	defer server.Close()

	t.Setenv("SEARCHIT_API_BASE", server.URL)

	oldVersion := version.Version
	version.Version = "v0.6.2-dev"
	defer func() { version.Version = oldVersion }()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	rootCmd.SetContext(context.Background())
	versionCmd.SetContext(context.Background())
	rootCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	versionCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	rootCmd.SetArgs([]string{"version"})

	execErr := rootCmd.ExecuteContext(context.Background())

	w.Close()
	os.Stdout = oldStdout

	var outBuf bytes.Buffer
	io.Copy(&outBuf, r)
	r.Close()

	if execErr != nil {
		t.Fatalf("version command failed: %v", execErr)
	}

	out := outBuf.String()
	if strings.Contains(out, "DOWNGRADE REQUESTED (WARNING)") {
		t.Errorf("version output should NOT contain 'DOWNGRADE REQUESTED (WARNING)', got:\n%s", out)
	}
	if !strings.Contains(out, "STATUS\n\n                UP TO DATE") {
		t.Errorf("expected version output to contain 'STATUS\\n\\n                UP TO DATE', got:\n%s", out)
	}
	if !strings.Contains(out, "CURRENT VERSION\n\n                v0.6.2-dev") {
		t.Errorf("expected output to contain CURRENT VERSION v0.6.2-dev, got:\n%s", out)
	}
	if !strings.Contains(out, "RECOMMENDED\n\n                v0.6.1") {
		t.Errorf("expected output to contain RECOMMENDED v0.6.1, got:\n%s", out)
	}
	if !strings.Contains(out, "NEXT MAJOR\n\n                v1.0.0") {
		t.Errorf("expected output to contain NEXT MAJOR v1.0.0, got:\n%s", out)
	}
}
