package wordlist

import (
	"context"
)

// LoadEffectiveWords loads the effective FUZZ vocabulary regardless of where it originates.
// If path is non-empty, it loads entries from the specified file.
// If path is empty, it loads entries from the default embedded wordlist.
// Callers do not need to distinguish whether the vocabulary originated from a file or the embedded fallback.
func LoadEffectiveWords(ctx context.Context, path string) ([]string, error) {
	var r Reader
	if path != "" {
		r = FileReader{Path: path}
	} else {
		r = EmbeddedReader{}
	}
	return LoadWords(ctx, r)
}
