package importer

import (
	"reflect"
	"strings"
	"testing"
)

// ─── Tokenize ────────────────────────────────────────────────────────────────

func TestTokenize_Basic(t *testing.T) {
	got, err := Tokenize("scan -t 128 --adaptive")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"scan", "-t", "128", "--adaptive"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestTokenize_DoubleQuotes(t *testing.T) {
	got, err := Tokenize(`scan --user-agent "My Bot/1.0"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"scan", "--user-agent", "My Bot/1.0"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestTokenize_SingleQuotes(t *testing.T) {
	got, err := Tokenize("scan -H 'Authorization: Bearer X'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"scan", "-H", "Authorization: Bearer X"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestTokenize_BackslashEscape(t *testing.T) {
	got, err := Tokenize(`scan --proxy http://127.0.0.1:8080\ path`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"scan", "--proxy", "http://127.0.0.1:8080 path"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestTokenize_EmptyInput(t *testing.T) {
	got, err := Tokenize("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestTokenize_ExtraWhitespace(t *testing.T) {
	got, err := Tokenize("  scan   -t   128  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"scan", "-t", "128"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestTokenize_UnterminatedSingleQuote(t *testing.T) {
	_, err := Tokenize("scan -H 'Missing close")
	if err == nil {
		t.Fatal("expected error for unterminated single quote, got nil")
	}
	if !strings.Contains(err.Error(), "single quote") {
		t.Errorf("error should mention single quote, got: %v", err)
	}
}

func TestTokenize_UnterminatedDoubleQuote(t *testing.T) {
	_, err := Tokenize(`scan --user-agent "Missing close`)
	if err == nil {
		t.Fatal("expected error for unterminated double quote, got nil")
	}
	if !strings.Contains(err.Error(), "double quote") {
		t.Errorf("error should mention double quote, got: %v", err)
	}
}

// ─── stripExecutable ─────────────────────────────────────────────────────────

func TestStripExecutable_WithExecutable(t *testing.T) {
	in := []string{"searchit", "scan", "-t", "128"}
	got := stripExecutable(in)
	want := []string{"scan", "-t", "128"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestStripExecutable_WithoutExecutable(t *testing.T) {
	in := []string{"scan", "-t", "128"}
	got := stripExecutable(in)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("should be unchanged; got %v", got)
	}
}

func TestStripExecutable_WindowsExe(t *testing.T) {
	in := []string{"searchit.exe", "scan", "--adaptive"}
	got := stripExecutable(in)
	want := []string{"scan", "--adaptive"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestStripExecutable_AbsolutePath(t *testing.T) {
	in := []string{"/usr/local/bin/searchit", "scan", "-t", "64"}
	got := stripExecutable(in)
	want := []string{"scan", "-t", "64"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v; want %v", got, want)
	}
}

func TestStripExecutable_RelativePath(t *testing.T) {
	in := []string{"./searchit", "fuzz", "--threads", "32"}
	got := stripExecutable(in)
	want := []string{"fuzz", "--threads", "32"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v; want %v", got, want)
	}
}

// ─── ParseCommand: empty / error cases ───────────────────────────────────────

func TestParseCommand_EmptyString(t *testing.T) {
	_, _, err := ParseCommand("scan", "", nil)
	if err == nil {
		t.Fatal("expected error for empty command string")
	}
}

func TestParseCommand_ExecutableOnly(t *testing.T) {
	_, _, err := ParseCommand("scan", "searchit", nil)
	if err == nil {
		t.Fatal("expected error when only executable is given without subcommand")
	}
}

func TestParseCommand_UnknownTool(t *testing.T) {
	_, _, err := ParseCommand("grep", "grep -t 128", nil)
	if err == nil {
		t.Fatal("expected error for unsupported tool")
	}
}

func TestParseCommand_NamespaceMismatch_ScanVsFuzz(t *testing.T) {
	// Fuzz command for a scan profile should be rejected.
	_, _, err := ParseCommand("scan", "searchit fuzz --threads 64", nil)
	if err == nil {
		t.Fatal("expected namespace mismatch error")
	}
	if !strings.Contains(err.Error(), "fuzz") || !strings.Contains(err.Error(), "scan") {
		t.Errorf("error should mention both tool names, got: %v", err)
	}
}

func TestParseCommand_NamespaceMismatch_FuzzVsScan(t *testing.T) {
	_, _, err := ParseCommand("fuzz", "scan -t 128 --adaptive", nil)
	if err == nil {
		t.Fatal("expected namespace mismatch error")
	}
}

func TestParseCommand_MalformedCommand_UnterminatedQuote(t *testing.T) {
	_, _, err := ParseCommand("scan", `scan --user-agent "Unclosed`, nil)
	if err == nil {
		t.Fatal("expected error for unterminated quote")
	}
}
