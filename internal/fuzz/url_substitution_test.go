package fuzz

import (
	"testing"
)

// TestURLSubstitution_EmbeddedPrefix verifies that static content around the FUZZ
// placeholder is always preserved during template rendering. This is the regression
// test for the reported bug where "/hmr_FUZZ" could lose the "hmr_" prefix.
func TestURLSubstitution_EmbeddedPrefix(t *testing.T) {
	tests := []struct {
		name     string
		template string
		fuzzVal  string
		wantURL  string
	}{
		{
			name:     "prefix before FUZZ is preserved",
			template: "http://example.com/hmr_FUZZ",
			fuzzVal:  "images",
			wantURL:  "http://example.com/hmr_images",
		},
		{
			name:     "prefix before FUZZ - css",
			template: "http://example.com/hmr_FUZZ",
			fuzzVal:  "css",
			wantURL:  "http://example.com/hmr_css",
		},
		{
			name:     "prefix before FUZZ - php extension variant",
			template: "http://example.com/hmr_FUZZ",
			fuzzVal:  "images.php",
			wantURL:  "http://example.com/hmr_images.php",
		},
		{
			name:     "FUZZ in middle of path segment",
			template: "http://example.com/api/FUZZ/test",
			fuzzVal:  "users",
			wantURL:  "http://example.com/api/users/test",
		},
		{
			name:     "FUZZ between two static segments",
			template: "http://example.com/prefix_FUZZ_suffix",
			fuzzVal:  "middle",
			wantURL:  "http://example.com/prefix_middle_suffix",
		},
		{
			name:     "FUZZ at end of URL - no prefix",
			template: "http://example.com/FUZZ",
			fuzzVal:  "admin",
			wantURL:  "http://example.com/admin",
		},
		{
			name:     "FUZZ with query param context",
			template: "http://example.com/search?q=FUZZ",
			fuzzVal:  "hello",
			wantURL:  "http://example.com/search?q=hello",
		},
		{
			name:     "multi-segment prefix path",
			template: "http://example.com/api/v2/hmr_FUZZ",
			fuzzVal:  "config",
			wantURL:  "http://example.com/api/v2/hmr_config",
		},
		{
			name:     "empty fuzz value preserves prefix",
			template: "http://example.com/hmr_FUZZ",
			fuzzVal:  "",
			wantURL:  "http://example.com/hmr_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test using GenCompiledTemplate (used by Generator.Generate)
			genTmpl := CompileGenTemplate(tt.template)
			var vals [5]string
			vals[PlaceholderFUZZ] = tt.fuzzVal
			gotGen := genTmpl.Render(vals)
			if gotGen != tt.wantURL {
				t.Errorf("CompileGenTemplate.Render: got %q, want %q", gotGen, tt.wantURL)
			}

			// Test using CompiledTemplate (used by Runner.BuildJob and IterateCandidates)
			compiledTmpl := CompileTemplate(tt.template, SupportedPlaceholders)
			vars := map[string]string{
				"FUZZ": tt.fuzzVal,
				"FOO":  "",
				"BAR":  "",
				"BAZ":  "",
				"BUZZ": "",
			}
			gotCompiled := compiledTmpl.RenderString(vars)
			if gotCompiled != tt.wantURL {
				t.Errorf("CompileTemplate.RenderString: got %q, want %q", gotCompiled, tt.wantURL)
			}
		})
	}
}

// TestURLSubstitution_StaticPrefixNeverDiscarded is a direct assertion that the
// "hmr_" prefix is never lost regardless of how the template is parsed or rendered.
// This is the canonical regression test for the reported user-facing bug.
func TestURLSubstitution_StaticPrefixNeverDiscarded(t *testing.T) {
	const urlTemplate = "http://10.81.163.78:1337/hmr_FUZZ"

	words := []string{"images", "css", "js"}
	extVariants := []struct {
		fuzzVal string
		want    string
	}{
		{"images", "http://10.81.163.78:1337/hmr_images"},
		{"images.php", "http://10.81.163.78:1337/hmr_images.php"},
		{"css", "http://10.81.163.78:1337/hmr_css"},
		{"js", "http://10.81.163.78:1337/hmr_js"},
	}
	_ = words

	genTmpl := CompileGenTemplate(urlTemplate)
	compiledTmpl := CompileTemplate(urlTemplate, SupportedPlaceholders)

	for _, tc := range extVariants {
		t.Run(tc.fuzzVal, func(t *testing.T) {
			// GenCompiledTemplate path
			var vals [5]string
			vals[PlaceholderFUZZ] = tc.fuzzVal
			got := genTmpl.Render(vals)
			if got != tc.want {
				t.Errorf("GenCompiledTemplate: got %q, want %q — static prefix 'hmr_' was lost", got, tc.want)
			}

			// CompiledTemplate path
			vars := map[string]string{"FUZZ": tc.fuzzVal}
			got2 := compiledTmpl.RenderString(vars)
			if got2 != tc.want {
				t.Errorf("CompiledTemplate: got %q, want %q — static prefix 'hmr_' was lost", got2, tc.want)
			}

			// Ensure the result does NOT look like the placeholder was simply replaced at the path root
			wrongURL := "http://10.81.163.78:1337/" + tc.fuzzVal
			if got == wrongURL {
				t.Errorf("GenCompiledTemplate: produced %q which looks like hmr_ was discarded", got)
			}
		})
	}
}

// TestURLSubstitution_MultiPlaceholder verifies that static content is preserved
// when multiple different placeholders are present in the same template.
func TestURLSubstitution_MultiPlaceholder(t *testing.T) {
	template := "http://example.com/api/FUZZ/items/FOO"
	compiled := CompileTemplate(template, SupportedPlaceholders)

	vars := map[string]string{
		"FUZZ": "users",
		"FOO":  "42",
	}
	got := compiled.RenderString(vars)
	want := "http://example.com/api/users/items/42"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestTruncateTemplate_EmbeddedPrefix tests that TruncateTemplate preserves the
// embedded prefix of the current level's placeholder during multi-level traversal.
func TestTruncateTemplate_EmbeddedPrefix(t *testing.T) {
	plan := TraversalPlan{
		Levels: []TraversalLevel{
			{Placeholder: "FUZZ", Words: []string{"images"}},
			{Placeholder: "FOO", Words: []string{"1"}},
		},
	}

	// At depth 0 (fuzzing FUZZ), the template should keep "hmr_FUZZ" and truncate FOO.
	got := plan.TruncateTemplate("http://example.com/hmr_FUZZ/FOO", 0)
	want := "http://example.com/hmr_FUZZ"
	if got != want {
		t.Errorf("depth 0: got %q, want %q", got, want)
	}

	// At depth 1 (last depth), it returns the full template.
	got1 := plan.TruncateTemplate("http://example.com/hmr_FUZZ/FOO", 1)
	want1 := "http://example.com/hmr_FUZZ/FOO"
	if got1 != want1 {
		t.Errorf("depth 1: got %q, want %q", got1, want1)
	}
}
