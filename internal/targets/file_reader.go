package targets

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// ReadFile reads target URLs from the specified file path.
// It trims whitespace, skips empty lines, and ignores lines starting with '#'.
func ReadFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseTargets(file)
}

// parseTargets reads target URLs from any io.Reader.
func parseTargets(r io.Reader) ([]string, error) {
	var urls []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		urls = append(urls, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return urls, nil
}
