package fuzz

import (
	"strings"
)

// Token represents a single part of a compiled template.
type Token struct {
	IsVar bool
	Value string
}

// CompiledTemplate is a sequence of tokens representing a parsed template.
type CompiledTemplate []Token

// CompileTemplate parses a template string into a CompiledTemplate.
// It searches for the given placeholders and splits the string into text and variable tokens.
func CompileTemplate(template string, placeholders []string) CompiledTemplate {
	if template == "" {
		return nil
	}

	if len(placeholders) == 0 {
		return CompiledTemplate{{IsVar: false, Value: template}}
	}

	var tokens CompiledTemplate

	// A naive but simple tokenizer for a fixed set of known placeholders.
	// We iteratively split the string.
	// To scale beyond current placeholders, we just search for the earliest placeholder.

	remaining := template
	for len(remaining) > 0 {
		firstIdx := -1
		firstVar := ""

		for _, ph := range placeholders {
			idx := strings.Index(remaining, ph)
			if idx != -1 {
				if firstIdx == -1 || idx < firstIdx {
					firstIdx = idx
					firstVar = ph
				}
			}
		}

		if firstIdx == -1 {
			// No more placeholders
			tokens = append(tokens, Token{IsVar: false, Value: remaining})
			break
		}

		if firstIdx > 0 {
			tokens = append(tokens, Token{IsVar: false, Value: remaining[:firstIdx]})
		}

		tokens = append(tokens, Token{IsVar: true, Value: firstVar})
		remaining = remaining[firstIdx+len(firstVar):]
	}

	return tokens
}

// Render writes the rendered template to a strings.Builder using the provided variable map.
func (c CompiledTemplate) Render(vars map[string]string, b *strings.Builder) {
	for _, t := range c {
		if t.IsVar {
			b.WriteString(vars[t.Value])
		} else {
			b.WriteString(t.Value)
		}
	}
}

// RenderString is a convenience method for rendering directly to a string.
func (c CompiledTemplate) RenderString(vars map[string]string) string {
	if len(c) == 0 {
		return ""
	}
	// Fast path for templates with no variables
	if len(c) == 1 && !c[0].IsVar {
		return c[0].Value
	}

	var b strings.Builder
	// Guess length
	b.Grow(64)
	c.Render(vars, &b)
	return b.String()
}
