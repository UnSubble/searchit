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
	// For production, the NEWS files might not be deployed with the binary unless embedded.
	// We'll read from the current working directory for development,
	// but normally you'd either embed them or fetch from GitHub.
	// Since the maintainer wants `NEWS/vX.Y.Z.md` as the source of truth,
	// we will look for it locally first.

	filename := filepath.Join(newsDir, fmt.Sprintf("%s.md", version))

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return News{}, fmt.Errorf("no news available for %s", version)
		}
		return News{}, err
	}

	return News{
		Version: version,
		Content: string(data),
	}, nil
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
