package extensions

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Parse takes a slice of raw extension arguments (e.g. ["php,txt", "@file.txt", "bak"])
// and returns a deduplicated, order-preserved slice of normalized extensions.
// Leading dots and surrounding whitespace are stripped from non-empty extensions.
// File arguments starting with '@' are read line-by-line.
//
// An empty string ("") is a valid extension meaning "the extensionless variant of
// the base word". It is preserved when explicitly produced by a leading, trailing,
// or consecutive comma in the input (e.g. ",php,html" → ["", "php", "html"]).
func Parse(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{})
	var result []string

	addExt := func(ext string) {
		// For non-empty tokens, strip leading dots and whitespace.
		// For empty tokens (from consecutive commas), preserve "" as-is.
		trimmed := strings.TrimSpace(ext)
		if trimmed != "" {
			trimmed = strings.TrimPrefix(trimmed, ".")
			trimmed = strings.TrimSpace(trimmed)
		}
		if _, exists := seen[trimmed]; !exists {
			seen[trimmed] = struct{}{}
			result = append(result, trimmed)
		}
	}

	for _, item := range raw {
		// A completely blank raw item (e.g. an empty -e flag with nothing at all)
		// is ignored. A raw item that contains commas may still produce "" entries.
		if strings.TrimSpace(item) == "" {
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

// GenerateVariants returns one candidate per extension for baseWord.
//
//   - If exts is nil or empty, returns []string{baseWord} (no extensions specified).
//   - If exts is non-empty, each entry determines one variant:
//   - ""    → baseWord          (extensionless; from an explicit empty entry like ",php")
//   - "php" → baseWord + ".php"
//
// The returned slice preserves the order of exts. No deduplication is performed
// here; callers rely on Parse to deduplicate before calling GenerateVariants.
func GenerateVariants(baseWord string, exts []string) []string {
	if len(exts) == 0 {
		return []string{baseWord}
	}
	variants := make([]string, 0, len(exts))
	for _, ext := range exts {
		if ext == "" {
			variants = append(variants, baseWord)
		} else {
			variants = append(variants, baseWord+"."+ext)
		}
	}
	return variants
}
