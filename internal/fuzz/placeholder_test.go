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

func TestDetectPlaceholderLocations(t *testing.T) {
	tests := []struct {
		name        string
		req         RequestTemplate
		placeholder string
		expected    []string
	}{
		{
			name: "URL path only",
			req: RequestTemplate{
				URL: "https://FUZZ.example.com/BAR",
			},
			placeholder: "FUZZ",
			expected:    []string{"URL"},
		},
		{
			name: "URL query parameter only",
			req: RequestTemplate{
				URL: "https://example.com/api?user=FOO",
			},
			placeholder: "FOO",
			expected:    []string{"Query parameter"},
		},
		{
			name: "URL and Query parameter together",
			req: RequestTemplate{
				URL: "https://FUZZ.example.com/api?user=FUZZ",
			},
			placeholder: "FUZZ",
			expected:    []string{"URL", "Query parameter"},
		},
		{
			name: "Header value",
			req: RequestTemplate{
				URL: "https://example.com",
				Headers: http.Header{
					"Host": []string{"BUZZ.futurevera.thm"},
				},
			},
			placeholder: "BUZZ",
			expected:    []string{"Header: Host"},
		},
		{
			name: "Header key",
			req: RequestTemplate{
				URL: "https://example.com",
				Headers: http.Header{
					"X-FOO-Header": []string{"value"},
				},
			},
			placeholder: "FOO",
			expected:    []string{"Header: X-FOO-Header"},
		},
		{
			name: "Multiple headers sorted alphabetically",
			req: RequestTemplate{
				URL: "https://example.com",
				Headers: http.Header{
					"Authorization": []string{"Bearer FOO"},
					"X-Forwarded":   []string{"FOO"},
				},
			},
			placeholder: "FOO",
			expected:    []string{"Header: Authorization", "Header: X-Forwarded"},
		},
		{
			name: "Body",
			req: RequestTemplate{
				URL:  "https://example.com",
				Body: `{"id":"BAZ"}`,
			},
			placeholder: "BAZ",
			expected:    []string{"Body"},
		},
		{
			name: "Cookie",
			req: RequestTemplate{
				URL:    "https://example.com",
				Cookie: "session=BAR",
			},
			placeholder: "BAR",
			expected:    []string{"Cookie"},
		},
		{
			name: "Multiple locations: URL, Header, Body, Cookie",
			req: RequestTemplate{
				URL:    "https://FUZZ.example.com/search?q=FUZZ",
				Body:   "payload=FUZZ",
				Cookie: "auth=FUZZ",
				Headers: http.Header{
					"Host": []string{"FUZZ.domain"},
				},
			},
			placeholder: "FUZZ",
			expected:    []string{"URL", "Query parameter", "Header: Host", "Body", "Cookie"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectPlaceholderLocations(tc.req, tc.placeholder)
			if len(got) != len(tc.expected) {
				t.Fatalf("expected %d locations (%v), got %d (%v)", len(tc.expected), tc.expected, len(got), got)
			}
			for i, v := range got {
				if v != tc.expected[i] {
					t.Errorf("at index %d, expected %q, got %q", i, tc.expected[i], v)
				}
			}
		})
	}
}

func TestGetPlaceholderLocations(t *testing.T) {
	req := RequestTemplate{
		URL: "https://FUZZ.example.com/api",
		Headers: http.Header{
			"Host": []string{"FUZZ.domain"},
		},
	}
	got := GetPlaceholderLocations(req, "FUZZ")
	expected := "URL, Header: Host"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}

	none := GetPlaceholderLocations(req, "BAR")
	if none != "None" {
		t.Errorf("expected %q, got %q", "None", none)
	}
}

func TestHasAnyPlaceholder(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"https://FUZZ.example.com", true},
		{"Host: FOO.example.com", true},
		{"session=BAR", true},
		{"data=BAZ", true},
		{"custom=BUZZ", true},
		{"https://example.com/api", false},
		{"Host: example.com", false},
		{"session=12345", false},
	}

	for _, tc := range tests {
		got := HasAnyPlaceholder(tc.input)
		if got != tc.expected {
			t.Errorf("HasAnyPlaceholder(%q) = %v, expected %v", tc.input, got, tc.expected)
		}
	}
}
