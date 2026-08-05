package scan_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/profile/scan"
	"github.com/unsubble/searchit/internal/profile/types"
	"gopkg.in/yaml.v3"
)

func intPtr(i int) *int                 { return &i }
func uint16Ptr(u uint16) *uint16        { return &u }
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

func TestScanValidator_Tool(t *testing.T) {
	v := scan.NewValidator()
	if v.Tool() != "scan" {
		t.Errorf("expected tool %q, got %q", "scan", v.Tool())
	}
}

func TestScanValidator_Validate(t *testing.T) {
	v := scan.NewValidator()

	tests := []struct {
		name    string
		overlay scan.Overlay
		wantErr bool
	}{
		{
			name: "Valid Complete Overlay",
			overlay: scan.Overlay{
				URL:           strPtr("http://example.com"),
				Threads:       intPtr(10),
				MaxDepth:      uint16Ptr(3),
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
			overlay: scan.Overlay{
				Threads: intPtr(0),
			},
			wantErr: true,
		},
		{
			name: "Invalid MaxDepth",
			overlay: scan.Overlay{
				MaxDepth: uint16Ptr(0),
			},
			wantErr: true,
		},
		{
			name: "Invalid Rate",
			overlay: scan.Overlay{
				Rate: floatPtr(0.0),
			},
			wantErr: true,
		},
		{
			name: "Invalid Strategy",
			overlay: scan.Overlay{
				Strategy: strPtr("invalid-strategy"),
			},
			wantErr: true,
		},
		{
			name: "Negative MaxRedirects",
			overlay: scan.Overlay{
				MaxRedirects: intPtr(-1),
			},
			wantErr: true,
		},
		{
			name: "Invalid MatchStatus",
			overlay: scan.Overlay{
				MatchStatus: strPtr("abc"),
			},
			wantErr: true,
		},
		{
			name: "Invalid ExcludeStatus",
			overlay: scan.Overlay{
				ExcludeStatus: strPtr("99999"),
			},
			wantErr: true,
		},
		{
			name: "Invalid IncludeSize",
			overlay: scan.Overlay{
				IncludeSize: strPtr("bad-size"),
			},
			wantErr: true,
		},
		{
			name: "Invalid ExcludeSize",
			overlay: scan.Overlay{
				ExcludeSize: strPtr("xyz-abc"),
			},
			wantErr: true,
		},
		{
			name: "Invalid MatchRegex",
			overlay: scan.Overlay{
				MatchRegex: strSlicePtr("[unclosed regex"),
			},
			wantErr: true,
		},
		{
			name: "Invalid FilterRegex",
			overlay: scan.Overlay{
				FilterRegex: strSlicePtr("[unclosed regex"),
			},
			wantErr: true,
		},
		{
			name: "Invalid Proxy",
			overlay: scan.Overlay{
				Proxy: strPtr(":%not-a-valid-url"),
			},
			wantErr: true,
		},
		{
			name: "Invalid URL",
			overlay: scan.Overlay{
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
