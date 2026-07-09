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
	v1 "github.com/unsubble/searchit/internal/profile/schema/v1"
	"gopkg.in/yaml.v3"
)

func init() {
	_ = profile.RegisterBuiltinValidators()
	_ = profile.RegisterBuiltinDecoders()
}

// scanConfig is a test-local struct that mirrors the fields used in
// embedded scan profiles. This proves that Decode works with any
// caller-supplied struct — the profile package itself does not need
// to know about this type.
type scanConfig struct {
	Threads       int    `yaml:"threads"`
	Timeout       int    `yaml:"timeout"`
	Recursive     bool   `yaml:"recursive"`
	MaxDepth      uint16 `yaml:"max-depth"`
	ExcludeStatus string `yaml:"exclude-status"`
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
	if p.Schema != 1 {
		t.Errorf("Schema = %d, want 1", p.Schema)
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

	overrideYAML := `schema: 1
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

	overrideYAML := `schema: 1
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
	// scan/deep is a true overlay — it does not include exclude-status,
	// so ExcludeStatus should remain at its zero value.
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

func TestCreateProfile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	store := &profile.DefaultStore{UserDir: tmpDir}

	var configNode yaml.Node
	configNode.Kind = yaml.MappingNode

	p := profile.Profile{
		Schema:      1,
		Name:        "scan/newprofile",
		Tool:        "scan",
		Description: "A brand new profile",
		Config:      configNode,
	}

	err := store.Create(p)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify file was created
	filePath := filepath.Join(tmpDir, "scan", "newprofile.yaml")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("profile file not found on disk: %v", err)
	}

	// Verify content by loading it back
	loaded, err := store.Load("scan/newprofile")
	if err != nil {
		t.Fatalf("failed to load newly created profile: %v", err)
	}

	if loaded.Name != "scan/newprofile" {
		t.Errorf("Name = %q, want 'scan/newprofile'", loaded.Name)
	}
	if loaded.Tool != "scan" {
		t.Errorf("Tool = %q, want 'scan'", loaded.Tool)
	}
	if loaded.Description != "A brand new profile" {
		t.Errorf("Description = %q, want 'A brand new profile'", loaded.Description)
	}
}

func TestCreateProfile_DuplicateUserProfile(t *testing.T) {
	tmpDir := t.TempDir()
	store := &profile.DefaultStore{UserDir: tmpDir}

	var configNode yaml.Node
	configNode.Kind = yaml.MappingNode

	p := profile.Profile{
		Schema: 1,
		Name:   "scan/dup",
		Tool:   "scan",
		Config: configNode,
	}

	if err := store.Create(p); err != nil {
		t.Fatalf("first creation failed: %v", err)
	}

	// Attempt duplicate creation
	err := store.Create(p)
	if err == nil {
		t.Fatal("expected duplicate creation to fail, but got no error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected error to contain 'already exists', got %q", err.Error())
	}
}

func TestCreateProfile_DuplicateEmbeddedProfile(t *testing.T) {
	tmpDir := t.TempDir()
	store := &profile.DefaultStore{UserDir: tmpDir}

	var configNode yaml.Node
	configNode.Kind = yaml.MappingNode

	// 'scan/quick' is built-in
	p := profile.Profile{
		Schema: 1,
		Name:   "scan/quick",
		Tool:   "scan",
		Config: configNode,
	}

	err := store.Create(p)
	if err == nil {
		t.Fatal("expected duplicate of built-in to fail, but got no error")
	}
	if !strings.Contains(err.Error(), "already exists as a built-in profile") {
		t.Errorf("expected error to contain 'already exists as a built-in profile', got %q", err.Error())
	}
}

func TestCreateProfile_InvalidNames(t *testing.T) {
	tmpDir := t.TempDir()
	store := &profile.DefaultStore{UserDir: tmpDir}

	var configNode yaml.Node
	configNode.Kind = yaml.MappingNode

	invalidNames := []string{
		"",           // empty name
		"scan",       // missing namespace
		"scan/",      // trailing slash
		"/scan/test", // leading slash
		"scan//test", // consecutive slashes
		"scan/.",     // dot
		"scan/..",    // dot dot
		"scan/a\\b",  // backslash
		"scan/a:b",   // colon
		"scan/a*b",   // asterisk
		"scan/a?b",   // question mark
		"scan/a\"b",  // quote
		"scan/a<b",   // angle bracket
		"scan/a>b",   // angle bracket
		"scan/a|b",   // pipe
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			p := profile.Profile{
				Schema: 1,
				Name:   name,
				Tool:   "scan",
				Config: configNode,
			}
			err := store.Create(p)
			if err == nil {
				t.Errorf("expected validation failure for name %q, but got nil", name)
			}
		})
	}
}

func TestCreateProfile_YAMLSerialization(t *testing.T) {
	tmpDir := t.TempDir()
	store := &profile.DefaultStore{UserDir: tmpDir}

	var configNode yaml.Node
	configNode.Kind = yaml.MappingNode

	p := profile.Profile{
		Schema:      1,
		Name:        "scan/testyaml",
		Tool:        "scan",
		Description: "testing YAML",
		Config:      configNode,
	}

	if err := store.Create(p); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	filePath := filepath.Join(tmpDir, "scan", "testyaml.yaml")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}

	// Verify exact YAML structure
	expectedLines := []string{
		"schema: 1",
		"name: scan/testyaml",
		"tool: scan",
		"description: testing YAML",
		"config: {}",
	}

	content := string(data)
	for _, line := range expectedLines {
		if !strings.Contains(content, line) {
			t.Errorf("expected YAML to contain %q, but got:\n%s", line, content)
		}
	}
}

func TestParseName(t *testing.T) {
	tests := []struct {
		input   string
		want    profile.Name
		wantErr bool
	}{
		{"scan/base", profile.Name{Tool: "scan", Name: "scan/base"}, false},
		{"fuzz/json-api", profile.Name{Tool: "fuzz", Name: "fuzz/json-api"}, false},
		{"subdomain/default", profile.Name{Tool: "subdomain", Name: "subdomain/default"}, false},
		{"", profile.Name{}, true},
		{"scan", profile.Name{}, true},
		{"/scan/test", profile.Name{}, true},
		{"scan/", profile.Name{}, true},
		{"scan//test", profile.Name{}, true},
		{"scan/.", profile.Name{}, true},
		{"scan/..", profile.Name{}, true},
		{"scan/a\\b", profile.Name{}, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := profile.ParseName(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("ParseName(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("ParseName(%q) = %+v, want %+v", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidate_GenericValidation(t *testing.T) {
	// Valid profile helper
	validProfile := func() profile.Profile {
		var node yaml.Node
		node.Kind = yaml.MappingNode
		return profile.Profile{
			Schema:      1,
			Name:        "scan/myprofile",
			Tool:        "scan",
			Description: "desc",
			Config:      node,
		}
	}

	t.Run("valid profile", func(t *testing.T) {
		p := validProfile()
		if err := profile.Validate(&p); err != nil {
			t.Fatalf("expected valid profile, got error: %v", err)
		}
	})

	t.Run("missing schema version", func(t *testing.T) {
		p := validProfile()
		p.Schema = 0
		if err := profile.Validate(&p); err == nil {
			t.Fatal("expected error for missing schema version, got nil")
		}
	})

	t.Run("unsupported schema version", func(t *testing.T) {
		p := validProfile()
		p.Schema = 2
		if err := profile.Validate(&p); err == nil {
			t.Fatal("expected error for unsupported schema version, got nil")
		}
	})

	t.Run("missing tool", func(t *testing.T) {
		p := validProfile()
		p.Tool = ""
		if err := profile.Validate(&p); err == nil {
			t.Fatal("expected error for missing tool, got nil")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		p := validProfile()
		p.Name = ""
		if err := profile.Validate(&p); err == nil {
			t.Fatal("expected error for missing name, got nil")
		}
	})

	t.Run("invalid namespace", func(t *testing.T) {
		p := validProfile()
		p.Name = "invalidname"
		if err := profile.Validate(&p); err == nil {
			t.Fatal("expected error for invalid name, got nil")
		}
	})

	t.Run("tool namespace mismatch", func(t *testing.T) {
		p := validProfile()
		p.Name = "fuzz/profile" // Tool is scan, name namespace is fuzz
		if err := profile.Validate(&p); err == nil {
			t.Fatal("expected error for namespace mismatch, got nil")
		}
	})

	t.Run("missing config node", func(t *testing.T) {
		p := validProfile()
		p.Config = yaml.Node{} // kind = 0
		if err := profile.Validate(&p); err == nil {
			t.Fatal("expected error for missing config, got nil")
		}
	})

	t.Run("invalid config node type (not mapping)", func(t *testing.T) {
		p := validProfile()
		p.Config.Kind = yaml.ScalarNode // scalar
		if err := profile.Validate(&p); err == nil {
			t.Fatal("expected error for non-mapping config, got nil")
		}
	})
}

type mockValidator struct {
	tool string
}

func (m *mockValidator) Tool() string {
	return m.tool
}

func (m *mockValidator) Validate(p *profile.Profile) error {
	return nil
}

func TestValidatorRegistry(t *testing.T) {
	t.Run("successful registration and lookup", func(t *testing.T) {
		mv := &mockValidator{tool: "mocktool"}
		err := profile.Register(mv)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		lookup := profile.GetValidator("mocktool")
		if lookup == nil {
			t.Fatal("GetValidator returned nil, expected mockValidator")
		}
		if lookup.Tool() != "mocktool" {
			t.Errorf("expected tool name 'mocktool', got %q", lookup.Tool())
		}
	})

	t.Run("duplicate registration fails", func(t *testing.T) {
		mv1 := &mockValidator{tool: "dup"}
		mv2 := &mockValidator{tool: "dup"}

		err := profile.Register(mv1)
		if err != nil {
			t.Fatalf("first registration failed: %v", err)
		}

		err = profile.Register(mv2)
		if err == nil {
			t.Fatal("expected duplicate registration to fail, but got nil")
		}
		if !strings.Contains(err.Error(), "already registered") {
			t.Errorf("expected error to mention 'already registered', got %q", err.Error())
		}
	})

	t.Run("missing validator returns nil", func(t *testing.T) {
		lookup := profile.GetValidator("nonexistent")
		if lookup != nil {
			t.Errorf("expected nil for nonexistent validator, got %v", lookup)
		}
	})

	t.Run("bootstrap registration", func(t *testing.T) {
		err := profile.RegisterBuiltinValidators()
		if err != nil {
			if !strings.Contains(err.Error(), "already registered") {
				t.Fatalf("RegisterBuiltinValidators failed unexpectedly: %v", err)
			}
		}

		scanVal := profile.GetValidator("scan")
		if scanVal == nil {
			t.Fatal("expected 'scan' validator to be registered after bootstrap")
		}
		if scanVal.Tool() != "scan" {
			t.Errorf("expected tool 'scan', got %q", scanVal.Tool())
		}
	})
}

func TestSchemaVersioning(t *testing.T) {
	t.Run("schema header detection", func(t *testing.T) {
		data := []byte(`schema: 1
name: test
tool: scan
`)
		var header profile.Header
		err := yaml.Unmarshal(data, &header)
		if err != nil {
			t.Fatalf("unmarshal header failed: %v", err)
		}
		if header.Schema != 1 {
			t.Errorf("expected schema 1, got %d", header.Schema)
		}
	})

	t.Run("decoder lookup", func(t *testing.T) {
		d, err := profile.GetDecoder(1)
		if err != nil {
			t.Fatalf("GetDecoder(1) failed: %v", err)
		}
		if d.Schema() != 1 {
			t.Errorf("expected decoder schema 1, got %d", d.Schema())
		}
	})

	t.Run("duplicate decoder registration", func(t *testing.T) {
		d1 := v1.New()
		err := profile.RegisterDecoder(d1)
		if err == nil {
			t.Fatal("expected duplicate decoder registration to fail, got nil")
		}
		if !strings.Contains(err.Error(), "already registered") {
			t.Errorf("expected duplicate error, got %v", err)
		}
	})

	t.Run("unsupported schema", func(t *testing.T) {
		_, err := profile.GetDecoder(999)
		if err == nil {
			t.Fatal("expected error for unsupported schema version 999, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported profile schema version") {
			t.Errorf("expected unsupported error, got %v", err)
		}
	})

	t.Run("successful v1 decoding", func(t *testing.T) {
		data := []byte(`schema: 1
name: scan/wordpress
tool: scan
description: Wordpress profile
config:
  threads: 10
`)
		p, err := profile.DecodeProfile(data)
		if err != nil {
			t.Fatalf("DecodeProfile failed: %v", err)
		}
		if p.Schema != 1 {
			t.Errorf("expected schema 1, got %d", p.Schema)
		}
		if p.Name != "scan/wordpress" {
			t.Errorf("expected name 'scan/wordpress', got %q", p.Name)
		}
		if p.Tool != "scan" {
			t.Errorf("expected tool 'scan', got %q", p.Tool)
		}
		if p.Description != "Wordpress profile" {
			t.Errorf("expected description, got %q", p.Description)
		}
	})

	t.Run("malformed header", func(t *testing.T) {
		data := []byte(`schema: : invalid`)
		_, err := profile.DecodeProfile(data)
		if err == nil {
			t.Fatal("expected malformed header to fail decoding, got nil")
		}
	})

	t.Run("runtime representation correctness", func(t *testing.T) {
		d, err := profile.GetDecoder(1)
		if err != nil {
			t.Fatalf("expected decoder v1 to exist: %v", err)
		}
		data := []byte(`schema: 1
name: scan/rt
tool: scan
description: rt desc
config:
  threads: 5
`)
		p, err := d.Decode(data)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if p.Schema != 1 {
			t.Errorf("expected schema 1, got %d", p.Schema)
		}
		if p.Name != "scan/rt" {
			t.Errorf("expected name, got %q", p.Name)
		}
		if p.Tool != "scan" {
			t.Errorf("expected tool, got %q", p.Tool)
		}
		if p.Description != "rt desc" {
			t.Errorf("expected description, got %q", p.Description)
		}
	})

	t.Run("bootstrap registration", func(t *testing.T) {
		err := profile.RegisterBuiltinDecoders()
		if err != nil {
			if !strings.Contains(err.Error(), "already registered") {
				t.Fatalf("expected RegisterBuiltinDecoders to succeed or return already registered, got: %v", err)
			}
		}
	})
}

func TestProfile_Metadata(t *testing.T) {
	t.Run("metadata decoding", func(t *testing.T) {
		data := []byte(`schema: 1
name: scan/meta
tool: scan
description: Meta desc
author: UnSubble
tags:
  - wordpress
  - php
homepage: https://github.com
license: MIT
created: "2026-07-07"
updated: "2026-07-09"
inherits:
  - scan/base
experimental: true
config:
  threads: 5
`)
		p, err := profile.DecodeProfile(data)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if p.Author != "UnSubble" {
			t.Errorf("expected author UnSubble, got %q", p.Author)
		}
		if len(p.Tags) != 2 || p.Tags[0] != "wordpress" || p.Tags[1] != "php" {
			t.Errorf("expected tags [wordpress, php], got %v", p.Tags)
		}
		if p.Homepage != "https://github.com" {
			t.Errorf("expected homepage, got %q", p.Homepage)
		}
		if p.License != "MIT" {
			t.Errorf("expected license, got %q", p.License)
		}
		if p.Created != "2026-07-07" {
			t.Errorf("expected created, got %q", p.Created)
		}
		if p.Updated != "2026-07-09" {
			t.Errorf("expected updated, got %q", p.Updated)
		}
		if len(p.Inherits) != 1 || p.Inherits[0] != "scan/base" {
			t.Errorf("expected inherits [scan/base], got %v", p.Inherits)
		}
		if !p.Experimental {
			t.Errorf("expected experimental true, got false")
		}
	})

	t.Run("empty metadata", func(t *testing.T) {
		data := []byte(`schema: 1
name: scan/empty
tool: scan
description: Empty desc
config:
  threads: 5
`)
		p, err := profile.DecodeProfile(data)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if p.Author != "" {
			t.Errorf("expected empty author, got %q", p.Author)
		}
		if len(p.Tags) != 0 {
			t.Errorf("expected empty tags, got %v", p.Tags)
		}
		if p.Homepage != "" {
			t.Errorf("expected empty homepage, got %q", p.Homepage)
		}
		if p.License != "" {
			t.Errorf("expected empty license, got %q", p.License)
		}
		if p.Created != "" {
			t.Errorf("expected empty created, got %q", p.Created)
		}
		if p.Updated != "" {
			t.Errorf("expected empty updated, got %q", p.Updated)
		}
		if len(p.Inherits) != 0 {
			t.Errorf("expected empty inherits, got %v", p.Inherits)
		}
		if p.Experimental {
			t.Errorf("expected experimental false, got true")
		}
	})
}
