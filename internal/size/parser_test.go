package size_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/size"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty input", "", false},
		{"single exact", "123", false},
		{"multiple exact", "123,456", false},
		{"single range", "100-200", false},
		{"range and exact", "123,200-300,456", false},
		{"zero size", "0", false},
		{"invalid integer", "abc", true},
		{"invalid range bounds", "200-100", true},
		{"missing range bound", "100-", true},
		{"negative number", "-100", true},
		{"double hyphen range", "100--200", true},
		{"empty token comma first", ",123", true},
		{"empty token comma last", "123,", true},
		{"empty token mid", "123,,456", true},
		{"leading zero", "0123", false},
		{"trailing whitespace", " 123 ", false},
		{"large integer overflow check", "99999999999999999999999999999999999999", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := size.Parse(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("Parse(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func FuzzParseSizeFilters(f *testing.F) {
	f.Add("100")
	f.Add("100-200,500,0")
	f.Add("xyz")
	f.Add("0")
	f.Add("0123")
	f.Add(" 100-200 ")
	f.Add("")

	f.Fuzz(func(t *testing.T, s string) {
		filters, err := size.Parse(s)
		if err == nil && len(filters) > 0 {
			for _, val := range []int64{-1, 0, 1, 150, 999999, 1<<62} {
				filters.Match(val)
			}
			_ = filters.String()
		}
	})
}
