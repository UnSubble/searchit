package fuzz_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/profile/fuzz"
	"github.com/unsubble/searchit/internal/profile/types"
	"gopkg.in/yaml.v3"
)

func intPtr(i int) *int                 { return &i }
func floatPtr(f float64) *float64       { return &f }
func strPtr(s string) *string           { return &s }
func strSlicePtr(s ...string) *[]string { return &s }

func createProfile(t *testing.T, overlay any) *types.Profile {
	t.Helper()
	data, err := yaml.Marshal(map[string]any{"config": overlay})
	if err != nil {
		t.Fatalf("failed to marshal overlay: %v", err)
	}
	var p types.Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		t.Fatalf("failed to unmarshal profile: %v", err)
	}
	return &p
}

func TestFuzzValidator_Tool(t *testing.T) {
	v := fuzz.NewValidator()
	if v.Tool() != "fuzz" {
		t.Errorf("expected tool %q, got %q", "fuzz", v.Tool())
	}
}

func TestFuzzValidator_Validate(t *testing.T) {
	v := fuzz.NewValidator()

	tests := []struct {
		name    string
		overlay fuzz.Overlay
		wantErr bool
	}{
		{
			name: "Valid Complete Overlay",
			overlay: fuzz.Overlay{
				URL:           strPtr("http://example.com/FUZZ"),
				Threads:       intPtr(10),
				Rate:          floatPtr(100.0),
				Strategy:      strPtr("bfs"),
				MaxRedirects:  intPtr(5),
				MatchStatus:   strPtr("200,301"),
				ExcludeStatus: strPtr("404"),
				IncludeSize:   strPtr("100-200"),
				ExcludeSize:   strPtr("0"),
				MatchRegex:    strSlicePtr("admin.*"),
				FilterRegex:   strSlicePtr("error.*"),
				Proxy:         strPtr("http://127.0.0.1:8080"),
			},
			wantErr: false,
		},
		{
			name: "Invalid Threads",
			overlay: fuzz.Overlay{
				Threads: intPtr(0),
			},
			wantErr: true,
		},
		{
			name: "Invalid Rate",
			overlay: fuzz.Overlay{
				Rate: floatPtr(0.0),
			},
			wantErr: true,
		},
		{
			name: "Invalid Strategy",
			overlay: fuzz.Overlay{
				Strategy: strPtr("invalid-strategy"),
			},
			wantErr: true,
		},
		{
			name: "Negative MaxRedirects",
			overlay: fuzz.Overlay{
				MaxRedirects: intPtr(-1),
			},
			wantErr: true,
		},
		{
			name: "Invalid MatchStatus",
			overlay: fuzz.Overlay{
				MatchStatus: strPtr("abc"),
			},
			wantErr: true,
		},
		{
			name: "Invalid ExcludeStatus",
			overlay: fuzz.Overlay{
				ExcludeStatus: strPtr("99999"),
			},
			wantErr: true,
		},
		{
			name: "Invalid IncludeSize",
			overlay: fuzz.Overlay{
				IncludeSize: strPtr("bad-size"),
			},
			wantErr: true,
		},
		{
			name: "Invalid ExcludeSize",
			overlay: fuzz.Overlay{
				ExcludeSize: strPtr("xyz-abc"),
			},
			wantErr: true,
		},
		{
			name: "Invalid MatchRegex",
			overlay: fuzz.Overlay{
				MatchRegex: strSlicePtr("[unclosed regex"),
			},
			wantErr: true,
		},
		{
			name: "Invalid FilterRegex",
			overlay: fuzz.Overlay{
				FilterRegex: strSlicePtr("[unclosed regex"),
			},
			wantErr: true,
		},
		{
			name: "Invalid Proxy",
			overlay: fuzz.Overlay{
				Proxy: strPtr(":%not-a-valid-url"),
			},
			wantErr: true,
		},
		{
			name: "Invalid URL",
			overlay: fuzz.Overlay{
				URL: strPtr(":%not-a-valid-url"),
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := createProfile(t, tc.overlay)
			err := v.Validate(p)
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}
