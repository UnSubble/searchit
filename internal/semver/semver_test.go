package semver

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		v       string
		want    Version
		wantErr bool
	}{
		{"stable_valid", "v0.5.0", Version{Original: "v0.5.0", Major: 0, Minor: 5, Patch: 0, PreRelease: ""}, false},
		{"stable_large", "v1.23.456", Version{Original: "v1.23.456", Major: 1, Minor: 23, Patch: 456, PreRelease: ""}, false},
		{"experimental_alpha", "v1.0.0-alpha", Version{Original: "v1.0.0-alpha", Major: 1, Minor: 0, Patch: 0, PreRelease: "alpha"}, false},
		{"experimental_beta", "v2.0.0-beta.1", Version{Original: "v2.0.0-beta.1", Major: 2, Minor: 0, Patch: 0, PreRelease: "beta.1"}, false},
		{"experimental_rc", "v1.0.0-rc1", Version{Original: "v1.0.0-rc1", Major: 1, Minor: 0, Patch: 0, PreRelease: "rc1"}, false},

		// Malformed
		{"missing_v", "0.5.0", Version{}, true},
		{"invalid_chars", "v0.a.0", Version{}, true},
		{"partial", "v1.0", Version{}, true},
		{"extra_dots", "v1.0.0.0", Version{}, true},
		{"garbage", "invalid", Version{}, true},
		{"garbage_v", "vhello", Version{}, true},
		{"empty", "", Version{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		v1   string
		v2   string
		want int
	}{
		{"v0.5.0", "v0.4.0", 1},
		{"v0.4.0", "v0.5.0", -1},
		{"v1.0.0", "v1.0.0", 0},
		{"v1.1.0", "v1.0.9", 1},
		{"v2.0.0", "v1.9.9", 1},

		// Prereleases are older than stables
		{"v1.0.0", "v1.0.0-beta", 1},
		{"v1.0.0-beta", "v1.0.0", -1},

		// Prerelease comparisons (lexicographical)
		{"v1.0.0-beta", "v1.0.0-alpha", 1},
		{"v1.0.0-alpha", "v1.0.0-beta", -1},
		{"v1.0.0-rc2", "v1.0.0-rc1", 1},
		{"v1.0.0-rc1", "v1.0.0-rc1", 0},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			v1, _ := Parse(tt.v1)
			v2, _ := Parse(tt.v2)
			if got := v1.Compare(v2); got != tt.want {
				t.Errorf("Compare() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChannelAndStable(t *testing.T) {
	v1, _ := Parse("v1.0.0")
	if !v1.IsStable() || v1.Channel() != "stable" {
		t.Errorf("v1.0.0 should be stable")
	}

	v2, _ := Parse("v1.0.0-beta")
	if v2.IsStable() || v2.Channel() != "experimental" {
		t.Errorf("v1.0.0-beta should be experimental")
	}
}

func TestBumpAndNextMajor(t *testing.T) {
	v, _ := Parse("v1.2.3")

	if got, _ := v.Bump("patch"); got != "v1.2.4" {
		t.Errorf("patch bump failed, got %v", got)
	}
	if got, _ := v.Bump("minor"); got != "v1.3.0" {
		t.Errorf("minor bump failed, got %v", got)
	}
	if got, _ := v.Bump("major"); got != "v2.0.0" {
		t.Errorf("major bump failed, got %v", got)
	}

	if got := v.NextMajor(); got != "v2.0.0" {
		t.Errorf("NextMajor failed, got %v", got)
	}
}
