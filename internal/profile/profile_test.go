package profile_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/profile"
)

// scanConfig is a test-local struct that mirrors the fields used in
// embedded scan profiles. This proves that Decode works with any
// caller-supplied struct — the profile package itself does not need
// to know about this type.
type scanConfig struct {
	Threads       int    `yaml:"threads"`
	Timeout       int    `yaml:"timeout"`
	Recursive     bool   `yaml:"recursive"`
	MaxDepth      uint16 `yaml:"max_depth"`
	ExcludeStatus string `yaml:"exclude_status"`
}

// ---------------------------------------------------------------------------
// Embedded profile loading
// ---------------------------------------------------------------------------

func TestLoadEmbeddedProfile_Base(t *testing.T) {
	store := profile.NewStore()
	p, err := store.Load("scan/base")
	if err != nil {
		t.Fatalf("Load(scan/base): %v", err)
	}
	if p.Name != "scan/base" {
		t.Errorf("Name = %q, want %q", p.Name, "scan/base")
	}
	if p.Tool != "scan" {
		t.Errorf("Tool = %q, want %q", p.Tool, "scan")
	}
	if p.Version != 1 {
		t.Errorf("Version = %d, want 1", p.Version)
	}

	var cfg scanConfig
	if err := p.Decode(&cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.Threads != 32 {
		t.Errorf("Threads = %d, want 32", cfg.Threads)
	}
}

func TestLoadEmbeddedProfile_Quick(t *testing.T) {
	store := profile.NewStore()
	p, err := store.Load("scan/quick")
	if err != nil {
		t.Fatalf("Load(scan/quick): %v", err)
	}
	if p.Name != "scan/quick" {
		t.Errorf("Name = %q, want %q", p.Name, "scan/quick")
	}

	var cfg scanConfig
	if err := p.Decode(&cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.Threads != 64 {
		t.Errorf("Threads = %d, want 64", cfg.Threads)
	}
	if cfg.Timeout != 5 {
		t.Errorf("Timeout = %d, want 5", cfg.Timeout)
	}
}

func TestLoadEmbeddedProfile_Deep(t *testing.T) {
	store := profile.NewStore()
	p, err := store.Load("scan/deep")
	if err != nil {
		t.Fatalf("Load(scan/deep): %v", err)
	}
	if p.Name != "scan/deep" {
		t.Errorf("Name = %q, want %q", p.Name, "scan/deep")
	}

	var cfg scanConfig
	if err := p.Decode(&cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.Threads != 16 {
		t.Errorf("Threads = %d, want 16", cfg.Threads)
	}
	if cfg.Timeout != 30 {
		t.Errorf("Timeout = %d, want 30", cfg.Timeout)
	}
	if !cfg.Recursive {
		t.Error("Recursive = false, want true")
	}
	if cfg.MaxDepth != 5 {
		t.Errorf("MaxDepth = %d, want 5", cfg.MaxDepth)
	}
}

// ---------------------------------------------------------------------------
// Missing profile
// ---------------------------------------------------------------------------

func TestLoadMissingProfile(t *testing.T) {
	store := profile.NewStore()
	_, err := store.Load("nonexistent/profile")
	if err == nil {
		t.Fatal("Load(nonexistent/profile): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Listing
// ---------------------------------------------------------------------------

func TestListProfiles(t *testing.T) {
	store := profile.NewStore()
	profiles, err := store.List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}

	if len(profiles) < 3 {
		t.Fatalf("List() returned %d profiles, want at least 3", len(profiles))
	}

	wantNames := []string{"scan/base", "scan/deep", "scan/quick"}
	var gotNames []string
	for _, p := range profiles {
		gotNames = append(gotNames, p.Name)
	}
	sort.Strings(gotNames)

	for _, want := range wantNames {
		found := false
		for _, got := range gotNames {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("List() missing profile %q; got %v", want, gotNames)
		}
	}
}

func TestListProfiles_AllBuiltin(t *testing.T) {
	store := profile.NewStore()
	profiles, err := store.List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}

	for _, p := range profiles {
		if !p.Builtin {
			t.Errorf("profile %q: Builtin = false, want true (no user dir configured)", p.Name)
		}
	}
}

func TestListProfiles_Sorted(t *testing.T) {
	store := profile.NewStore()
	profiles, err := store.List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}

	for i := 1; i < len(profiles); i++ {
		if profiles[i].Name < profiles[i-1].Name {
			t.Errorf("List() not sorted: %q comes after %q", profiles[i].Name, profiles[i-1].Name)
		}
	}
}

// ---------------------------------------------------------------------------
// Namespace handling
// ---------------------------------------------------------------------------

func TestNamespaceHandling(t *testing.T) {
	store := profile.NewStore()
	profiles, err := store.List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}

	for _, p := range profiles {
		if !strings.Contains(p.Name, "/") {
			t.Errorf("profile %q: expected namespaced name containing '/'", p.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// User override
// ---------------------------------------------------------------------------

func TestUserOverrideEmbedded(t *testing.T) {
	tmpDir := t.TempDir()
	scanDir := filepath.Join(tmpDir, "scan")
	if err := os.MkdirAll(scanDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	overrideYAML := `version: 1
name: scan/base
tool: scan
description: User-overridden base profile

config:
  threads: 128
  timeout: 60
`
	if err := os.WriteFile(filepath.Join(scanDir, "base.yaml"), []byte(overrideYAML), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	store := &profile.DefaultStore{UserDir: tmpDir}
	p, err := store.Load("scan/base")
	if err != nil {
		t.Fatalf("Load(scan/base): %v", err)
	}

	if p.Description != "User-overridden base profile" {
		t.Errorf("Description = %q, want %q", p.Description, "User-overridden base profile")
	}

	var cfg scanConfig
	if err := p.Decode(&cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.Threads != 128 {
		t.Errorf("Threads = %d, want 128", cfg.Threads)
	}
	if cfg.Timeout != 60 {
		t.Errorf("Timeout = %d, want 60", cfg.Timeout)
	}
}

func TestUserOverride_ListShowsUserNotBuiltin(t *testing.T) {
	tmpDir := t.TempDir()
	scanDir := filepath.Join(tmpDir, "scan")
	if err := os.MkdirAll(scanDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	overrideYAML := `version: 1
name: scan/base
tool: scan
description: User-overridden base profile

config:
  threads: 128
  timeout: 60
`
	if err := os.WriteFile(filepath.Join(scanDir, "base.yaml"), []byte(overrideYAML), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	store := &profile.DefaultStore{UserDir: tmpDir}
	profiles, err := store.List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}

	for _, p := range profiles {
		if p.Name == "scan/base" {
			if p.Builtin {
				t.Error("scan/base: Builtin = true, want false (user override should win)")
			}
			return
		}
	}
	t.Error("scan/base not found in List() results")
}

// ---------------------------------------------------------------------------
// LoadRaw
// ---------------------------------------------------------------------------

func TestLoadRaw(t *testing.T) {
	store := profile.NewStore()
	raw, err := store.LoadRaw("scan/quick")
	if err != nil {
		t.Fatalf("LoadRaw(scan/quick): %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("LoadRaw returned empty bytes")
	}
	s := string(raw)
	if !strings.Contains(s, "scan/quick") {
		t.Errorf("LoadRaw output missing profile name; got:\n%s", s)
	}
}

func TestLoadRaw_MissingProfile(t *testing.T) {
	store := profile.NewStore()
	_, err := store.LoadRaw("nonexistent")
	if err == nil {
		t.Fatal("LoadRaw(nonexistent): expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Decode
// ---------------------------------------------------------------------------

func TestDecode_PopulatesStruct(t *testing.T) {
	store := profile.NewStore()
	p, err := store.Load("scan/deep")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var cfg scanConfig
	if err := p.Decode(&cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if cfg.Threads != 16 {
		t.Errorf("Threads = %d, want 16", cfg.Threads)
	}
	if cfg.Timeout != 30 {
		t.Errorf("Timeout = %d, want 30", cfg.Timeout)
	}
	if !cfg.Recursive {
		t.Error("Recursive = false, want true")
	}
	if cfg.MaxDepth != 5 {
		t.Errorf("MaxDepth = %d, want 5", cfg.MaxDepth)
	}
	if cfg.ExcludeStatus != "404" {
		t.Errorf("ExcludeStatus = %q, want %q", cfg.ExcludeStatus, "404")
	}
}

func TestDecode_ArbitraryStruct(t *testing.T) {
	// Decode into a partial struct — only take fields you care about.
	store := profile.NewStore()
	p, err := store.Load("scan/quick")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	type partial struct {
		Threads int `yaml:"threads"`
	}
	var cfg partial
	if err := p.Decode(&cfg); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.Threads != 64 {
		t.Errorf("Threads = %d, want 64", cfg.Threads)
	}
}

func TestDecode_IntoMap(t *testing.T) {
	store := profile.NewStore()
	p, err := store.Load("scan/base")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var m map[string]any
	if err := p.Decode(&m); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	threads, ok := m["threads"]
	if !ok {
		t.Fatal("map missing 'threads' key")
	}
	if threads != 32 {
		t.Errorf("threads = %v, want 32", threads)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestUserDir_NonExistent(t *testing.T) {
	store := &profile.DefaultStore{UserDir: "/tmp/searchit-nonexistent-profile-dir-test"}
	profiles, err := store.List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(profiles) < 3 {
		t.Errorf("List() returned %d profiles, want at least 3 embedded", len(profiles))
	}
}

func TestEmbeddedProfiles_ToolField(t *testing.T) {
	store := profile.NewStore()
	profiles, err := store.List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	for _, p := range profiles {
		if p.Tool == "" {
			t.Errorf("profile %q: Tool is empty", p.Name)
		}
	}
}

func TestEmbeddedProfiles_DescriptionField(t *testing.T) {
	store := profile.NewStore()
	profiles, err := store.List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	for _, p := range profiles {
		if p.Description == "" {
			t.Errorf("profile %q: Description is empty", p.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// Architectural constraint: no internal/config dependency
// ---------------------------------------------------------------------------

func TestNoConfigImport(t *testing.T) {
	// Parse all Go source files in the profile package (excluding tests)
	// and verify that none import internal/config.
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse profile package: %v", err)
	}

	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			for _, imp := range file.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if strings.Contains(path, "internal/config") {
					t.Errorf("%s imports %q — profile package must not depend on internal/config", filename, path)
				}
			}
		}
	}
}
