package fuzz

import (
	"testing"
)

func BenchmarkCompileAndRender_Simple(b *testing.B) {
	tmpl := "http://example.com/FUZZ"
	cTmpl := CompileTemplate(tmpl, []string{"FUZZ", "FOO", "BAR", "BUZZ"})
	vars := map[string]string{"FUZZ": "test1"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cTmpl.RenderString(vars)
	}
}

func BenchmarkBuildJob_Simple(b *testing.B) {
	r := &Runner{
		Method: "GET",
		HeaderTemplates: map[string][]string{
			"User-Agent": {"searchit/1.0"},
		},
		CookieTemplate: "",
	}
	tmpl := "http://example.com/api/v1/FUZZ"
	r.TargetURL = tmpl
	r.compiledReq = r.compileRequest()
	vars := map[string]string{"FUZZ": "fuzzVal"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.buildJob(r.compiledReq.targetURL, vars)
	}
}
