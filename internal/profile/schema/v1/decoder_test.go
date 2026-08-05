package v1_test

import (
	"testing"

	v1 "github.com/unsubble/searchit/internal/profile/schema/v1"
)

func TestDecoder_V1(t *testing.T) {
	d := v1.New()
	if d.Schema() != 1 {
		t.Errorf("expected schema 1, got %d", d.Schema())
	}

	raw := `
schema: 1
name: quick-scan
tool: scan
description: Fast scanning profile
author: Searchit
tags: [fast, web]
homepage: https://searchit.example.com
license: MIT
created: "2026-01-01"
updated: "2026-02-01"
depends: ["scan/base"]
experimental: false
config:
  threads: 128
`

	prof, err := d.Decode([]byte(raw))
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if prof.Schema != 1 || prof.Name != "quick-scan" || prof.Tool != "scan" {
		t.Errorf("unexpected profile: %+v", prof)
	}
	if len(prof.Tags) != 2 || len(prof.Depends) != 1 {
		t.Errorf("tags or depends mismatch: tags=%v, depends=%v", prof.Tags, prof.Depends)
	}

	// Invalid YAML
	_, err = d.Decode([]byte("schema: [invalid yaml"))
	if err == nil {
		t.Fatal("expected decode error on malformed YAML")
	}
}
