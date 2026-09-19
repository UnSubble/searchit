package fuzz

import (
	"strings"
	"testing"
)

func TestEncoder_Noop(t *testing.T) {
	enc, err := NewEncoder("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enc.Name() != "" {
		t.Errorf("expected empty name, got %q", enc.Name())
	}
	input := "admin/user.php?q=1 2"
	if got := enc.Encode(input); got != input {
		t.Errorf("expected %q, got %q", input, got)
	}
}

func TestEncoder_Base64(t *testing.T) {
	enc, err := NewEncoder("base64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enc.Name() != "base64" {
		t.Errorf("expected name 'base64', got %q", enc.Name())
	}

	tests := []struct {
		input string
		want  string
	}{
		{"admin/user", "YWRtaW4vdXNlcg=="},
		{"admin", "YWRtaW4="},
		{"", ""},
		{"hello world", "aGVsbG8gd29ybGQ="},
		{"user@domain.com", "dXNlckBkb21haW4uY29t"},
	}

	for _, tt := range tests {
		got := enc.Encode(tt.input)
		if got != tt.want {
			t.Errorf("Base64(%q): got %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEncoder_URL(t *testing.T) {
	enc, err := NewEncoder("url")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enc.Name() != "url" {
		t.Errorf("expected name 'url', got %q", enc.Name())
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"path with slash", "admin/user", "admin%2Fuser"},
		{"extension preserved", "test.php", "test.php"},
		{"dot not encoded", "admin.user.test", "admin.user.test"},
		{"unreserved chars", "a-z_A-Z.0-9~", "a-z_A-Z.0-9~"},
		{"spaces encoded as %20", "admin user", "admin%20user"},
		{"plus sign encoded as %2B", "c++", "c%2B%2B"},
		{"query chars", "?id=1&name=test", "%3Fid%3D1%26name%3Dtest"},
		{"hash and at", "#anchor@host", "%23anchor%40host"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enc.Encode(tt.input)
			if got != tt.want {
				t.Errorf("URL(%q): got %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEncoder_DoubleURL(t *testing.T) {
	enc, err := NewEncoder("doubleurl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enc.Name() != "doubleurl" {
		t.Errorf("expected name 'doubleurl', got %q", enc.Name())
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"slash double encoded", "admin/user", "admin%252Fuser"},
		{"extension preserved", "test.php", "test.php"},
		{"space double encoded", "admin user", "admin%2520user"},
		{"query double encoded", "?id=1", "%253Fid%253D1"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enc.Encode(tt.input)
			if got != tt.want {
				t.Errorf("DoubleURL(%q): got %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEncoder_ValidationAndCase(t *testing.T) {
	// Case-insensitivity
	for _, name := range []string{"BASE64", "Base64", "URL", "Url", "DOUBLEURL", "DoubleUrl"} {
		enc, err := NewEncoder(name)
		if err != nil {
			t.Errorf("expected %q to be valid, got error: %v", name, err)
		}
		if enc == nil {
			t.Errorf("expected non-nil encoder for %q", name)
		}
	}

	// Invalid encodings
	invalidNames := []string{"hex", "rot13", "utf-8", "random", "none"}
	for _, inv := range invalidNames {
		enc, err := NewEncoder(inv)
		if err == nil {
			t.Errorf("expected error for invalid encoding %q, got nil", inv)
		}
		if enc != nil {
			t.Errorf("expected nil encoder for invalid encoding %q", inv)
		}
		if !strings.Contains(err.Error(), "supported encodings are base64, url, doubleurl") {
			t.Errorf("error message does not list supported encodings: %v", err)
		}
	}
}
