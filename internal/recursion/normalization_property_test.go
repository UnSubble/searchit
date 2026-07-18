package recursion

import (
	"math/rand"
	"strings"
	"testing"
)

// generateRandomURL constructs a random URL using a pool of components.
func generateRandomURL(rng *rand.Rand) string {
	schemes := []string{"http", "https", ""}
	hosts := []string{"example.com", "localhost", "127.0.0.1", "foo-bar.org", ""}
	paths := []string{"/admin", "/api/v1/", "/index.html", "/a/b/c/", "/a//b///c", "/a/./b/../c", ""}
	queries := []string{"?x=1", "?a=b&c=d", "?", ""}
	fragments := []string{"#section-1", "#", ""}

	scheme := schemes[rng.Intn(len(schemes))]
	host := hosts[rng.Intn(len(hosts))]
	path := paths[rng.Intn(len(paths))]
	query := queries[rng.Intn(len(queries))]
	fragment := fragments[rng.Intn(len(fragments))]

	var sb strings.Builder
	if scheme != "" {
		sb.WriteString(scheme + "://")
	}
	sb.WriteString(host)
	sb.WriteString(path)
	sb.WriteString(query)
	sb.WriteString(fragment)

	return sb.String()
}

func TestProperty_URLNormalization(t *testing.T) {
	rng := rand.New(rand.NewSource(42)) // Seeded RNG for reproducibility

	for i := 0; i < 10000; i++ {
		urlStr := generateRandomURL(rng)

		// Property 1: Determinism (Running twice must yield identical results)
		out1 := normalizeURL(urlStr)
		out2 := normalizeURL(urlStr)
		if out1 != out2 {
			t.Errorf("Determinism violated for input %q:\nRun 1: %q\nRun 2: %q", urlStr, out1, out2)
		}

		// Property 2: Idempotency (normalizeURL(normalizeURL(u)) == normalizeURL(u))
		out3 := normalizeURL(out1)
		if out1 != out3 {
			t.Errorf("Idempotency violated for input %q:\nFirst pass:  %q\nSecond pass: %q", urlStr, out1, out3)
		}

		// Property 3: Fragment stripping
		if strings.Contains(out1, "#") {
			t.Errorf("Fragment stripping property violated for input %q: output %q contains '#'", urlStr, out1)
		}

		// Property 4: Trailing slash normalization
		// We expect trailing slashes to be stripped in path context (unless it is a root slash e.g. "http://example.com/").
		if strings.HasSuffix(out1, "/") {
			// Count slashes to see if it is a root slash or directory path
			slashes := 0
			for j := 0; j < len(out1); j++ {
				if out1[j] == '/' {
					slashes++
				}
			}
			// If it's a deep path with more than 3 slashes, it shouldn't end with a slash
			if slashes > 3 && !strings.Contains(out1, "?") {
				t.Errorf("Trailing slash normalization property violated for input %q: output %q has trailing slash", urlStr, out1)
			}
		}
	}
}
