package wordlist

import (
	"bufio"
	"context"
	"os"
	"strings"
	"sync/atomic"

	"github.com/unsubble/searchit/internal/stats"
)

// FileReader streams entries from a local text file.
type FileReader struct {
	Path string
}

func (r FileReader) Read(ctx context.Context, out chan<- string) error {
	defer func() {
		atomic.AddInt64(&stats.GlobalInstrumentation.ReaderExit, 1)
		stats.GlobalInstrumentation.LogEvent("reader exited")
	}()

	file, err := os.Open(r.Path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
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
