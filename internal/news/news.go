package news

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var newsDir = "NEWS"

func SetNewsDir(dir string) {
	newsDir = dir
}

func GetNewsDir() string {
	return newsDir
}

type News struct {
	Version string
	Content string
}

// Fetch reads the news file for a given version.
func Fetch(version string) (News, error) {
	targetFile := fmt.Sprintf("%s.md", version)
	filename := filepath.Join(newsDir, targetFile)

	data, err := os.ReadFile(filename)
	if err == nil {
		return News{
			Version: version,
			Content: string(data),
		}, nil
	}
	if !os.IsNotExist(err) {
		return News{}, err
	}

	// If not found and newsDir is relative, attempt to search parent directories from cwd
	if !filepath.IsAbs(newsDir) {
		if cwd, err := os.Getwd(); err == nil {
			dir := cwd
			for {
				candidate := filepath.Join(dir, newsDir, targetFile)
				if data, err := os.ReadFile(candidate); err == nil {
					return News{
						Version: version,
						Content: string(data),
					}, nil
				}
				parent := filepath.Dir(dir)
				if parent == dir {
					break
				}
				dir = parent
			}
		}

		// Also check relative to executable
		if exe, err := os.Executable(); err == nil {
			if exePath, err := filepath.EvalSymlinks(exe); err == nil {
				dir := filepath.Dir(exePath)
				for {
					candidate := filepath.Join(dir, newsDir, targetFile)
					if data, err := os.ReadFile(candidate); err == nil {
						return News{
							Version: version,
							Content: string(data),
						}, nil
					}
					parent := filepath.Dir(dir)
					if parent == dir {
						break
					}
					dir = parent
				}
			}
		}
	}

	return News{}, fmt.Errorf("no news available for %s", version)
}

// FormatPreview formats a news preview block.
func FormatPreview(version string, title string, newItems, improvedItems, fixedItems []string) string {
	var builder strings.Builder

	builder.WriteString("--------------------------------------------------\n\n")
	builder.WriteString("                NEWS PREVIEW\n\n")
	builder.WriteString("--------------------------------------------------\n\n\n")

	builder.WriteString("VERSION\n\n")
	builder.WriteString(fmt.Sprintf("        %s\n\n\n", version))

	builder.WriteString("TITLE\n\n")
	builder.WriteString(fmt.Sprintf("        %s\n\n\n", title))

	if len(newItems) > 0 {
		builder.WriteString("--------------------------------------------------\n\n\n")
		builder.WriteString("NEW\n\n")
		for _, item := range newItems {
			builder.WriteString(fmt.Sprintf("        - %s\n", item))
		}
		builder.WriteString("\n\n")
	}

	if len(improvedItems) > 0 {
		builder.WriteString("--------------------------------------------------\n\n\n")
		builder.WriteString("IMPROVED\n\n")
		for _, item := range improvedItems {
			builder.WriteString(fmt.Sprintf("        - %s\n", item))
		}
		builder.WriteString("\n\n")
	}

	if len(fixedItems) > 0 {
		builder.WriteString("--------------------------------------------------\n\n\n")
		builder.WriteString("FIXED\n\n")
		for _, item := range fixedItems {
			builder.WriteString(fmt.Sprintf("        - %s\n", item))
		}
		builder.WriteString("\n\n")
	}

	return builder.String()
}
