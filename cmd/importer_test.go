package cmd

import (
	"reflect"
	"testing"

	"github.com/spf13/pflag"

	"github.com/unsubble/searchit/internal/profile/importer"
	"gopkg.in/yaml.v3"
)

func hasKey(node yaml.Node, key string) bool {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return true
		}
	}
	return false
}

// Helper to extract a scalar value from a yaml.MappingNode
func scalarVal(node yaml.Node, key string) (string, bool) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1].Value, true
		}
	}
	return "", false
}

// Helper to extract a sequence (list) of strings from a yaml.MappingNode
func seqVals(node yaml.Node, key string) ([]string, bool) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			seqNode := node.Content[i+1]
			if seqNode.Kind != yaml.SequenceNode {
				return nil, false
			}
			var vals []string
			for _, item := range seqNode.Content {
				vals = append(vals, item.Value)
			}
			return vals, true
		}
	}
	return nil, false
}

// ─── ParseCommand: scan ───────────────────────────────────────────────────────

func TestParseCommand_Scan_BasicFlags(t *testing.T) {
	cfg, warns, err := importer.ParseCommand("scan", "scan -t 128 --adaptive --random-agent", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}

	if v, ok := scalarVal(cfg, "threads"); !ok || v != "128" {
		t.Errorf("threads: got %q ok=%v; want 128", v, ok)
	}
	if v, ok := scalarVal(cfg, "adaptive"); !ok || v != "true" {
		t.Errorf("adaptive: got %q ok=%v; want true", v, ok)
	}
	if v, ok := scalarVal(cfg, "random-agent"); !ok || v != "true" {
		t.Errorf("random-agent: got %q ok=%v; want true", v, ok)
	}
}

func TestParseCommand_Scan_WithExecutable(t *testing.T) {
	cfg, _, err := importer.ParseCommand("scan", "searchit scan -t 64 --proxy http://127.0.0.1:8080", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := scalarVal(cfg, "threads"); !ok || v != "64" {
		t.Errorf("threads: got %q; want 64", v)
	}
	if v, ok := scalarVal(cfg, "proxy"); !ok || v != "http://127.0.0.1:8080" {
		t.Errorf("proxy: got %q; want http://127.0.0.1:8080", v)
	}
}

func TestParseCommand_Scan_WithoutExecutable(t *testing.T) {
	cfg, _, err := importer.ParseCommand("scan", "scan --user-agent TestBot/1.0 --timeout 5", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := scalarVal(cfg, "user-agent"); !ok || v != "TestBot/1.0" {
		t.Errorf("user-agent: got %q; want TestBot/1.0", v)
	}
	if v, ok := scalarVal(cfg, "timeout"); !ok || v != "5" {
		t.Errorf("timeout: got %q; want 5", v)
	}
}

func TestParseCommand_Scan_AbsoluteExePath(t *testing.T) {
	cfg, _, err := importer.ParseCommand("scan", "/usr/local/bin/searchit scan -t 16", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := scalarVal(cfg, "threads"); !ok || v != "16" {
		t.Errorf("threads: got %q; want 16", v)
	}
}

func TestParseCommand_Scan_ShortFlags(t *testing.T) {
	cfg, _, err := importer.ParseCommand("scan", "scan -t 32 -r -q", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := scalarVal(cfg, "threads"); !ok || v != "32" {
		t.Errorf("threads: got %q; want 32", v)
	}
	if v, ok := scalarVal(cfg, "recursive"); !ok || v != "true" {
		t.Errorf("recursive: got %q; want true", v)
	}
	if v, ok := scalarVal(cfg, "quiet"); !ok || v != "true" {
		t.Errorf("quiet: got %q; want true", v)
	}
}

func TestParseCommand_Scan_SliceFlags(t *testing.T) {
	cfg, _, err := importer.ParseCommand("scan", "scan --ext php --ext html --mr admin --mr login", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	exts, ok := seqVals(cfg, "ext")
	if !ok {
		t.Error("ext key not found")
	} else if !reflect.DeepEqual(exts, []string{"php", "html"}) {
		t.Errorf("ext: got %v; want [php html]", exts)
	}
	regexes, ok := seqVals(cfg, "match-regex")
	if !ok {
		t.Error("match-regex key not found")
	} else if !reflect.DeepEqual(regexes, []string{"admin", "login"}) {
		t.Errorf("match-regex: got %v; want [admin login]", regexes)
	}
}

func TestParseCommand_Scan_HeaderFlag(t *testing.T) {
	cfg, _, err := importer.ParseCommand("scan", `scan -H "Authorization: Bearer token"`, func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vals, ok := seqVals(cfg, "headers")
	if !ok {
		t.Error("headers key not found")
	} else if len(vals) == 0 || vals[0] != "Authorization: Bearer token" {
		t.Errorf("headers: got %v; want [Authorization: Bearer token]", vals)
	}
}

func TestParseCommand_Scan_DurationFlags(t *testing.T) {
	cfg, _, err := importer.ParseCommand("scan", "scan --delay 500ms --connect-timeout 5s", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := scalarVal(cfg, "delay"); !ok || v != "500ms" {
		t.Errorf("delay: got %q; want 500ms", v)
	}
	if v, ok := scalarVal(cfg, "connect-timeout"); !ok || v != "5s" {
		t.Errorf("connect-timeout: got %q; want 5s", v)
	}
}

func TestParseCommand_Scan_RateFlag(t *testing.T) {
	cfg, _, err := importer.ParseCommand("scan", "scan --rate 10.5", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := scalarVal(cfg, "rate"); !ok || v != "10.5" {
		t.Errorf("rate: got %q; want 10.5", v)
	}
}

// ─── ParseCommand: alias priority ─────────────────────────────────────────────

func TestParseCommand_Scan_AliasFC_OverridesExcludeStatus(t *testing.T) {
	// --fc has priority 1, --exclude-status has priority 0.
	// When both are present, --fc should win.
	cfg, _, err := importer.ParseCommand("scan", "scan --exclude-status 404 --fc 500", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, ok := scalarVal(cfg, "exclude-status")
	if !ok {
		t.Fatal("exclude-status key not found")
	}
	if v != "500" {
		t.Errorf("exclude-status: got %q; want 500 (fc should override exclude-status)", v)
	}
}

func TestParseCommand_Scan_AliasMC(t *testing.T) {
	cfg, _, err := importer.ParseCommand("scan", "scan --mc 200,301", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := scalarVal(cfg, "match-status"); !ok || v != "200,301" {
		t.Errorf("match-status: got %q ok=%v; want 200,301", v, ok)
	}
}

func TestParseCommand_Scan_AliasMS_OverridesIncludeSize(t *testing.T) {
	cfg, _, err := importer.ParseCommand("scan", "scan --include-size 100-200 --ms 512", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := scalarVal(cfg, "include-size"); !ok || v != "512" {
		t.Errorf("include-size: got %q; want 512 (ms should win)", v)
	}
}

// ─── ParseCommand: runtime-only flags ────────────────────────────────────────

func TestParseCommand_Scan_RuntimeFlagsGenerateWarnings(t *testing.T) {
	cfg, warns, err := importer.ParseCommand("scan", "scan -t 128 --no-progress --profile scan/quick --tech laravel -o out.json", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// threads should still be captured
	if v, ok := scalarVal(cfg, "threads"); !ok || v != "128" {
		t.Errorf("threads: got %q; want 128", v)
	}
	// runtime flags must NOT appear in YAML
	for _, key := range []string{"no-progress", "profile", "tech", "output"} {
		if hasKey(cfg, key) {
			t.Errorf("runtime flag %q should not appear in config YAML", key)
		}
	}
	// but we should get warnings
	if len(warns) == 0 {
		t.Error("expected warnings for runtime flags, got none")
	}
}

func TestParseCommand_Fuzz_RuntimeFlagsGenerateWarnings(t *testing.T) {
	cfg, warns, err := importer.ParseCommand("fuzz", "fuzz --threads 32 --no-progress --foo wordlist.txt --bar words2.txt", func() *pflag.FlagSet {
		cmd, _ := NewFuzzCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := scalarVal(cfg, "threads"); !ok || v != "32" {
		t.Errorf("threads: got %q; want 32", v)
	}
	for _, key := range []string{"no-progress", "foo", "bar"} {
		if hasKey(cfg, key) {
			t.Errorf("runtime flag %q should not appear in YAML", key)
		}
	}
	if len(warns) == 0 {
		t.Error("expected warnings for runtime flags")
	}
}

// ─── ParseCommand: fuzz ───────────────────────────────────────────────────────

func TestParseCommand_Fuzz_Basic(t *testing.T) {
	cfg, _, err := importer.ParseCommand("fuzz", "searchit fuzz --strategy eager --threads 64", func() *pflag.FlagSet {
		cmd, _ := NewFuzzCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := scalarVal(cfg, "strategy"); !ok || v != "eager" {
		t.Errorf("strategy: got %q; want eager", v)
	}
	if v, ok := scalarVal(cfg, "threads"); !ok || v != "64" {
		t.Errorf("threads: got %q; want 64", v)
	}
}

func TestParseCommand_Fuzz_ShortDataFlag(t *testing.T) {
	// fuzz uses -d for data (scan uses --data without short)
	cfg, _, err := importer.ParseCommand("fuzz", `fuzz -d "key=value"`, func() *pflag.FlagSet {
		cmd, _ := NewFuzzCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := scalarVal(cfg, "data"); !ok || v != "key=value" {
		t.Errorf("data: got %q; want key=value", v)
	}
}

func TestParseCommand_Fuzz_NoRecursionFields(t *testing.T) {
	// Fuzz has no recursive/max-depth/recurse-on — those flags should be rejected.
	_, _, err := importer.ParseCommand("fuzz", "fuzz --recursive", func() *pflag.FlagSet {
		cmd, _ := NewFuzzCmd()
		return cmd.Flags()
	})
	if err == nil {
		t.Error("expected error for --recursive on fuzz")
	}
}

// ─── Type Mapping ─────────────────────────────────────────────────────────────

func TestParseCommand_Scan_TypeMapping(t *testing.T) {
	cfg, _, err := importer.ParseCommand("scan", "scan -t 128 --rate 1.5 --adaptive --user-agent Bot", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	findNode := func(key string) *yaml.Node {
		for i := 0; i+1 < len(cfg.Content); i += 2 {
			if cfg.Content[i].Value == key {
				return cfg.Content[i+1]
			}
		}
		return nil
	}

	if n := findNode("threads"); n == nil || n.Tag != "!!int" {
		t.Errorf("threads should have !!int tag, got %v", n)
	}
	if n := findNode("rate"); n == nil || n.Tag != "!!float" {
		t.Errorf("rate should have !!float tag, got %v", n)
	}
	if n := findNode("adaptive"); n == nil || n.Tag != "!!bool" {
		t.Errorf("adaptive should have !!bool tag, got %v", n)
	}
	if n := findNode("user-agent"); n == nil || n.Tag != "!!str" {
		t.Errorf("user-agent should have !!str tag, got %v", n)
	}
}

// ─── ParseCommand: no flags → empty config ────────────────────────────────────

func TestParseCommand_Scan_NoFlags_EmptyConfig(t *testing.T) {
	cfg, _, err := importer.ParseCommand("scan", "scan", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kind != yaml.MappingNode {
		t.Errorf("expected MappingNode, got kind=%d", cfg.Kind)
	}
	if len(cfg.Content) != 0 {
		t.Errorf("expected empty mapping, got %d entries", len(cfg.Content)/2)
	}
}

// ─── ParseCommand: YAML roundtrip ────────────────────────────────────────────

// TestParseCommand_Scan_Roundtrip verifies that the produced yaml.Node can be
// marshaled and unmarshaled without loss.
func TestParseCommand_Scan_Roundtrip(t *testing.T) {
	cfg, _, err := importer.ParseCommand("scan", "scan -t 128 --timeout 5 --adaptive --user-agent TestBot --proxy http://127.0.0.1:8080", func() *pflag.FlagSet {
		cmd, _ := NewScanCmd()
		return cmd.Flags()
	})
	if err != nil {
		t.Fatalf("ParseCommand: %v", err)
	}

	type doc struct {
		Config yaml.Node `yaml:"config"`
	}
	d := doc{Config: cfg}
	data, err := yaml.Marshal(d)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	var d2 doc
	if err := yaml.Unmarshal(data, &d2); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	wantPairs := map[string]string{
		"threads":    "128",
		"timeout":    "5",
		"adaptive":   "true",
		"user-agent": "TestBot",
		"proxy":      "http://127.0.0.1:8080",
	}
	for key, want := range wantPairs {
		got, ok := scalarVal(d2.Config, key)
		if !ok || got != want {
			t.Errorf("after roundtrip, %s: got %q ok=%v; want %q", key, got, ok, want)
		}
	}
}
