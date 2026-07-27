package fuzz

import (
	"net/http"
	"testing"
)

func TestFindPlaceholders(t *testing.T) {
	tests := []struct {
		name     string
		req      RequestTemplate
		expected []string
	}{
		{
			name: "no placeholders",
			req: RequestTemplate{
				URL:    "http://example.com/api",
				Method: "GET",
			},
			expected: nil,
		},
		{
			name: "placeholder in URL",
			req: RequestTemplate{
				URL:    "http://example.com/api/FUZZ",
				Method: "GET",
			},
			expected: []string{"FUZZ"},
		},
		{
			name: "placeholder in Header key",
			req: RequestTemplate{
				URL:    "http://example.com",
				Method: "GET",
				Headers: http.Header{
					"X-FOO-Header": []string{"value"},
				},
			},
			expected: []string{"FOO"},
		},
		{
			name: "placeholder in Header value",
			req: RequestTemplate{
				URL:    "http://example.com",
				Method: "GET",
				Headers: http.Header{
					"User-Agent": []string{"MyApp/BUZZ"},
				},
			},
			expected: []string{"BUZZ"},
		},
		{
			name: "placeholder in Cookie",
			req: RequestTemplate{
				URL:    "http://example.com",
				Method: "GET",
				Cookie: "session=FUZZ",
			},
			expected: []string{"FUZZ"},
		},
		{
			name: "placeholder in Body",
			req: RequestTemplate{
				URL:    "http://example.com",
				Method: "POST",
				Body:   `{"id": "BAR"}`,
			},
			expected: []string{"BAR"},
		},
		{
			name: "multiple placeholders",
			req: RequestTemplate{
				URL:    "http://example.com/FUZZ",
				Method: "POST",
				Body:   `{"id": "BAR"}`,
				Headers: http.Header{
					"X-Custom": []string{"BUZZ"},
				},
			},
			expected: []string{"FUZZ", "BAR", "BUZZ"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FindPlaceholders(tc.req)
			if len(result) != len(tc.expected) {
				t.Fatalf("expected %d placeholders, got %d", len(tc.expected), len(result))
			}
			for i, v := range result {
				if v != tc.expected[i] {
					t.Errorf("expected placeholder %q at index %d, got %q", tc.expected[i], i, v)
				}
			}
		})
	}
}
