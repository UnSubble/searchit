package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// runCreate is a helper that runs "profile create" with the given extra args and
// returns the combined stdout/stderr and the first error.
func runCreate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return runProfileCommand(append([]string{"profile", "create"}, args...))
}

// readProfileConfig reads the created profile file and returns its config yaml.Node.
func readProfileConfig(t *testing.T, profileName string) yaml.Node {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	path := filepath.Join(home, ".config", "searchit", "profiles", profileName+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	var doc struct {
		Config yaml.Node `yaml:"config"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	return doc.Config
}

func configScalar(cfg yaml.Node, key string) (string, bool) {
	for i := 0; i+1 < len(cfg.Content); i += 2 {
		if cfg.Content[i].Value == key {
			return cfg.Content[i+1].Value, true
		}
	}
	return "", false
}

// ─── Empty profile ────────────────────────────────────────────────────────────

func TestProfileCreate_EmptyProfile_Scan(t *testing.T) {
	setupTestHome(t)
	out, err := runCreate(t, "scan/empty-test")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "scan/empty-test") {
		t.Errorf("output should contain profile name; got: %s", out)
	}

	cfg := readProfileConfig(t, "scan/empty-test")
	if len(cfg.Content) != 0 {
		t.Errorf("expected empty config, got %d entries", len(cfg.Content)/2)
	}
}

func TestProfileCreate_EmptyProfile_Fuzz(t *testing.T) {
	setupTestHome(t)
	out, err := runCreate(t, "fuzz/empty-test")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "fuzz/empty-test") {
		t.Errorf("output should contain profile name; got: %s", out)
	}
}

// ─── Import: scan ─────────────────────────────────────────────────────────────

func TestProfileCreate_ScanImport_BasicFlags(t *testing.T) {
	setupTestHome(t)
	out, err := runCreate(t, "scan/perf", "searchit scan -t 128 --adaptive --random-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}

	cfg := readProfileConfig(t, "scan/perf")

	if v, ok := configScalar(cfg, "threads"); !ok || v != "128" {
		t.Errorf("threads: got %q ok=%v; want 128", v, ok)
	}
	if v, ok := configScalar(cfg, "adaptive"); !ok || v != "true" {
		t.Errorf("adaptive: got %q ok=%v; want true", v, ok)
	}
	if v, ok := configScalar(cfg, "random-agent"); !ok || v != "true" {
		t.Errorf("random-agent: got %q ok=%v; want true", v, ok)
	}
	// Success output should mention imported fields count
	if !strings.Contains(out, "Imported:") {
		t.Errorf("output should mention imported fields count; got: %s", out)
	}
}

func TestProfileCreate_ScanImport_ExecutablePresent(t *testing.T) {
	setupTestHome(t)
	_, err := runCreate(t, "scan/with-exe",
		"searchit scan -t 64 --proxy http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := readProfileConfig(t, "scan/with-exe")
	if v, ok := configScalar(cfg, "threads"); !ok || v != "64" {
		t.Errorf("threads: got %q; want 64", v)
	}
	if v, ok := configScalar(cfg, "proxy"); !ok || v != "http://127.0.0.1:8080" {
		t.Errorf("proxy: got %q; want http://127.0.0.1:8080", v)
	}
}

func TestProfileCreate_ScanImport_ExecutableAbsent(t *testing.T) {
	setupTestHome(t)
	_, err := runCreate(t, "scan/no-exe",
		`scan --user-agent "Custom Bot/1.0" --timeout 5`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := readProfileConfig(t, "scan/no-exe")
	if v, ok := configScalar(cfg, "user-agent"); !ok || v != "Custom Bot/1.0" {
		t.Errorf("user-agent: got %q; want Custom Bot/1.0", v)
	}
	if v, ok := configScalar(cfg, "timeout"); !ok || v != "5" {
		t.Errorf("timeout: got %q; want 5", v)
	}
}

func TestProfileCreate_ScanImport_Equivalence(t *testing.T) {
	// CLI flag names should map to the YAML key names documented in the overlay.
	setupTestHome(t)
	_, err := runCreate(t, "scan/equiv",
		"scan --threads 32 --delay 500ms --rate 2.5 --follow-redirects --max-redirects 5 "+
			"--exclude-status 404,403 --proxy http://p.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := readProfileConfig(t, "scan/equiv")

	checks := map[string]string{
		"threads":          "32",
		"delay":            "500ms",
		"rate":             "2.5",
		"follow-redirects": "true",
		"max-redirects":    "5",
		"exclude-status":   "404,403",
		"proxy":            "http://p.example.com",
	}
	for key, want := range checks {
		got, ok := configScalar(cfg, key)
		if !ok || got != want {
			t.Errorf("%s: got %q ok=%v; want %q", key, got, ok, want)
		}
	}
}

// ─── Import: fuzz ─────────────────────────────────────────────────────────────

func TestProfileCreate_FuzzImport_BasicFlags(t *testing.T) {
	setupTestHome(t)
	_, err := runCreate(t, "fuzz/api",
		"searchit fuzz --strategy eager --threads 64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := readProfileConfig(t, "fuzz/api")
	if v, ok := configScalar(cfg, "strategy"); !ok || v != "eager" {
		t.Errorf("strategy: got %q; want eager", v)
	}
	if v, ok := configScalar(cfg, "threads"); !ok || v != "64" {
		t.Errorf("threads: got %q; want 64", v)
	}
}

func TestProfileCreate_FuzzImport_ExecutableAbsent(t *testing.T) {
	setupTestHome(t)
	_, err := runCreate(t, "fuzz/no-exe", "fuzz -t 32 --adaptive")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := readProfileConfig(t, "fuzz/no-exe")
	if v, ok := configScalar(cfg, "threads"); !ok || v != "32" {
		t.Errorf("threads: got %q; want 32", v)
	}
	if v, ok := configScalar(cfg, "adaptive"); !ok || v != "true" {
		t.Errorf("adaptive: got %q; want true", v)
	}
}

// ─── Error cases ──────────────────────────────────────────────────────────────

func TestProfileCreate_NoArgs_Error(t *testing.T) {
	setupTestHome(t)
	out, err := runCreate(t)
	if err == nil {
		t.Fatal("expected error for no arguments, got nil")
	}
	_ = out
}

func TestProfileCreate_DirectFlagError(t *testing.T) {
	setupTestHome(t)
	// User passes scan flags directly (not wrapped in quotes).
	// With FParseErrWhitelist, cobra passes these as extra positional args.
	// RunE should detect this and show a helpful error.
	_, err := runCreate(t, "scan/test", "-t", "128")
	if err == nil {
		t.Fatal("expected error when scan flags are passed directly")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "quoted string") {
		t.Errorf("error should suggest using a quoted string; got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "searchit scan") {
		t.Errorf("error should show an example command; got: %s", errMsg)
	}
}

func TestProfileCreate_NamespaceMismatch(t *testing.T) {
	setupTestHome(t)
	// Fuzz command for a scan profile.
	_, err := runCreate(t, "scan/mismatch", "searchit fuzz --threads 64")
	if err == nil {
		t.Fatal("expected error for namespace mismatch")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "fuzz") || !strings.Contains(errMsg, "scan") {
		t.Errorf("error should mention both tool names; got: %s", errMsg)
	}
}

func TestProfileCreate_MalformedCommand(t *testing.T) {
	setupTestHome(t)
	_, err := runCreate(t, "scan/broken", `scan --user-agent "Unclosed`)
	if err == nil {
		t.Fatal("expected error for unterminated quote in command")
	}
}

func TestProfileCreate_AlreadyExists(t *testing.T) {
	setupTestHome(t)
	// Create once — should succeed.
	if _, err := runCreate(t, "scan/dup"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	// Create again — should fail.
	_, err := runCreate(t, "scan/dup")
	if err == nil {
		t.Fatal("expected error when creating a duplicate profile")
	}
}

func TestProfileCreate_InvalidName_NoNamespace(t *testing.T) {
	setupTestHome(t)
	_, err := runCreate(t, "no-namespace")
	if err == nil {
		t.Fatal("expected error for name without namespace")
	}
}

// ─── Runtime-only flags produce warnings, not errors ─────────────────────────

func TestProfileCreate_RuntimeFlagsWarned(t *testing.T) {
	setupTestHome(t)
	// --no-progress and --output are runtime-only; they should produce warnings
	// but NOT cause an error or appear in the YAML.
	out, err := runCreate(t, "scan/warnings-test",
		"scan -t 64 --no-progress --output /tmp/out.json")
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
	cfg := readProfileConfig(t, "scan/warnings-test")
	if v, ok := configScalar(cfg, "threads"); !ok || v != "64" {
		t.Errorf("threads: got %q; want 64", v)
	}
	for _, runtimeKey := range []string{"no-progress", "output"} {
		for i := 0; i < len(cfg.Content); i += 2 {
			if cfg.Content[i].Value == runtimeKey {
				t.Errorf("runtime flag %q should not be in YAML config", runtimeKey)
			}
		}
	}
}

// ─── Help text ────────────────────────────────────────────────────────────────

func TestProfileCreate_HelpText(t *testing.T) {
	out, _ := runProfileCommand([]string{"profile", "create", "--help"})
	wantPhrases := []string{
		"empty template",
		"existing Searchit command",
		"searchit profile create",
		"scan/performance",
		"fuzz/api",
		"searchit profile show",
	}
	for _, phrase := range wantPhrases {
		if !strings.Contains(out, phrase) {
			t.Errorf("help text missing %q\n\ngot:\n%s", phrase, out)
		}
	}
}

// ─── Profile file is written on disk after import ────────────────────────────

func TestProfileCreate_ImportedProfileWrittenToDisk(t *testing.T) {
	home := setupTestHome(t)
	if _, err := runCreate(t, "scan/ondisk",
		"scan -t 128 --adaptive --user-agent TestBot/1.0"); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Verify the profile file exists at the canonical location.
	profilePath := filepath.Join(home, ".config", "searchit", "profiles", "scan", "ondisk.yaml")
	if _, err := os.Stat(profilePath); err != nil {
		t.Errorf("expected profile file at %s, got: %v", profilePath, err)
	}

	// Config must be non-empty (we imported 3 fields).
	cfg := readProfileConfig(t, "scan/ondisk")
	if len(cfg.Content) == 0 {
		t.Error("expected non-empty config after import")
	}
}
