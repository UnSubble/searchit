package wordlist

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"strings"
	"sync/atomic"

	"github.com/unsubble/searchit/internal/stats"
)

//go:embed embedded/common.txt
var embeddedCommon []byte

// EmbeddedReader streams entries from the bundled common.txt.
// It is intended for development and smoke testing, not production assessments.
type EmbeddedReader struct{}

func (EmbeddedReader) Read(ctx context.Context, out chan<- string) error {
	defer func() {
		atomic.AddInt64(&stats.GlobalInstrumentation.ReaderExit, 1)
		stats.GlobalInstrumentation.LogEvent("reader exited")
	}()

	scanner := bufio.NewScanner(bytes.NewReader(embeddedCommon))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		atomic.AddInt64(&stats.GlobalInstrumentation.WordsRead, 1)
		select {
		case <-ctx.Done():
			stats.GlobalInstrumentation.LogEvent("context cancellation")
			return ctx.Err()
		case out <- line:
		}
	}
	if scanner.Err() == nil {
		atomic.AddInt64(&stats.GlobalInstrumentation.ReaderEOF, 1)
	}
	return scanner.Err()
}
