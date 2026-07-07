package profile_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/profile"
)

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
	if p.Config.Threads != 32 {
		t.Errorf("Config.Threads = %d, want 32", p.Config.Threads)
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
	if p.Config.Threads != 64 {
		t.Errorf("Config.Threads = %d, want 64", p.Config.Threads)
	}
	if p.Config.Timeout != 5 {
		t.Errorf("Config.Timeout = %d, want 5", p.Config.Timeout)
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
	if p.Config.Threads != 16 {
		t.Errorf("Config.Threads = %d, want 16", p.Config.Threads)
	}
	if p.Config.Timeout != 30 {
		t.Errorf("Config.Timeout = %d, want 30", p.Config.Timeout)
	}
	if !p.Config.Recursive {
		t.Error("Config.Recursive = false, want true")
	}
	if p.Config.MaxDepth != 5 {
		t.Errorf("Config.MaxDepth = %d, want 5", p.Config.MaxDepth)
	}
}

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

func TestListProfiles(t *testing.T) {
	store := profile.NewStore()
	profiles, err := store.List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}

	// We expect at least the 3 embedded profiles.
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

func TestUserOverrideEmbedded(t *testing.T) {
	// Create a temporary user profile directory with a scan/base.yaml override.
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

	// User override should win.
	if p.Description != "User-overridden base profile" {
		t.Errorf("Description = %q, want %q", p.Description, "User-overridden base profile")
	}
	if p.Config.Threads != 128 {
		t.Errorf("Config.Threads = %d, want 128", p.Config.Threads)
	}
	if p.Config.Timeout != 60 {
		t.Errorf("Config.Timeout = %d, want 60", p.Config.Timeout)
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

func TestUserDir_NonExistent(t *testing.T) {
	store := &profile.DefaultStore{UserDir: "/tmp/searchit-nonexistent-profile-dir-test"}
	profiles, err := store.List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	// Should still return embedded profiles.
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
