package wordlist_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/wordlist"
)

func BenchmarkJoin(b *testing.B) {
	base := "https://example.com/admin/v2"
	word := "login/dashboard"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = wordlist.Join(base, word)
	}
}
