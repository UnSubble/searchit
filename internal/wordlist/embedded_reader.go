package wordlist

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"strings"
)

//go:embed embedded/common.txt
var embeddedCommon []byte

// EmbeddedReader streams entries from the bundled common.txt.
// It is intended for development and smoke testing, not production assessments.
type EmbeddedReader struct{}

func (EmbeddedReader) Read(ctx context.Context, out chan<- string) error {
	scanner := bufio.NewScanner(bytes.NewReader(embeddedCommon))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- line:
		}
	}
	return scanner.Err()
}
