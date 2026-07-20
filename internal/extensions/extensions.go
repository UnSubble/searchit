package extensions

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Parse takes a slice of raw extension arguments (e.g. ["php,txt", "@file.txt", "bak"])
// and returns a deduplicated, order-preserved slice of normalized extensions.
// Leading dots and surrounding whitespace are stripped.
// File arguments starting with '@' are read line-by-line.
func Parse(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{})
	var result []string

	addExt := func(ext string) {
		ext = strings.TrimSpace(ext)
		ext = strings.TrimPrefix(ext, ".")
		ext = strings.TrimSpace(ext)
		if ext == "" {
			return
		}
		if _, exists := seen[ext]; !exists {
			seen[ext] = struct{}{}
			result = append(result, ext)
		}
	}

	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		if strings.HasPrefix(item, "@") {
			filePath := strings.TrimPrefix(item, "@")
			filePath = strings.TrimSpace(filePath)
			if filePath == "" {
				return nil, fmt.Errorf("empty file path for --ext @")
			}
			file, err := os.Open(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to read extension file %q: %w", filePath, err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.Split(line, ",")
				for _, part := range parts {
					addExt(part)
				}
			}
			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("error reading extension file %q: %w", filePath, err)
			}
		} else {
			parts := strings.Split(item, ",")
			for _, part := range parts {
				addExt(part)
			}
		}
	}

	return result, nil
}

// GenerateVariants returns baseWord followed by baseWord + "." + ext for each extension.
// If exts is empty, it returns []string{baseWord}.
func GenerateVariants(baseWord string, exts []string) []string {
	if len(exts) == 0 {
		return []string{baseWord}
	}
	variants := make([]string, 0, 1+len(exts))
	variants = append(variants, baseWord)
	for _, ext := range exts {
		if ext == "" {
			continue
		}
		variants = append(variants, baseWord+"."+ext)
	}
	return variants
}
