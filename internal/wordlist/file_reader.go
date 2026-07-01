package wordlist

import (
	"bufio"
	"context"
	"os"
	"strings"
)

// FileReader streams entries from a local text file.
type FileReader struct {
	Path string
}

func (r FileReader) Read(ctx context.Context, out chan<- string) error {
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- line:
		}
	}
	return scanner.Err()
}
