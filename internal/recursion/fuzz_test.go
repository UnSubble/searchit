package recursion

import "testing"

func FuzzRecursionStrategyAndNormalization(f *testing.F) {
	f.Add("bfs")
	f.Add("DFS")
	f.Add("http://a.com/")
	f.Add("invalid")

	f.Fuzz(func(t *testing.T, s string) {
		_, _ = ParseStrategy(s)
		_ = normalizeURL(s)
	})
}
