package fuzz

import (
	"strings"
)

// Placeholder represents a strongly-typed enum for template variables.
type Placeholder uint8

const (
	PlaceholderFUZZ Placeholder = iota
	PlaceholderFOO
	PlaceholderBAR
	PlaceholderBAZ
	PlaceholderBUZZ
)

// SegmentKind represents the type of a parsed template segment.
type SegmentKind uint8

const (
	SegmentLiteral SegmentKind = iota
	SegmentPlaceholder
)

// GenTemplateSegment is a single chunk of a parsed template string.
type GenTemplateSegment struct {
	Kind SegmentKind
	Text string
	ID   Placeholder
}

// GenCompiledTemplate represents a template parsed once to optimize runtime replacement.
type GenCompiledTemplate struct {
	Segments         []GenTemplateSegment
	LiteralBytes     int
	PlaceholderCount [5]uint8
}

// GenCompiledHeader represents an HTTP header where both the key and values are compiled templates.
type GenCompiledHeader struct {
	Key    GenCompiledTemplate
	Values []GenCompiledTemplate
}

// CompileGenTemplate parses an input string into a GenCompiledTemplate.
// It is a pure function that allocates no global state and has no side effects.
func CompileGenTemplate(input string) GenCompiledTemplate {
	if input == "" {
		return GenCompiledTemplate{}
	}

	if !strings.Contains(input, "FUZZ") &&
		!strings.Contains(input, "FOO") &&
		!strings.Contains(input, "BAR") &&
		!strings.Contains(input, "BAZ") &&
		!strings.Contains(input, "BUZZ") {
		return GenCompiledTemplate{
			Segments:     []GenTemplateSegment{{Kind: SegmentLiteral, Text: input}},
			LiteralBytes: len(input),
		}
	}

	var ct GenCompiledTemplate
	i := 0
	literalStart := 0

	for i < len(input) {
		if input[i] == 'F' {
			if strings.HasPrefix(input[i:], "FUZZ") {
				if i > literalStart {
					text := input[literalStart:i]
					ct.Segments = append(ct.Segments, GenTemplateSegment{Kind: SegmentLiteral, Text: text})
					ct.LiteralBytes += len(text)
				}
				ct.Segments = append(ct.Segments, GenTemplateSegment{Kind: SegmentPlaceholder, ID: PlaceholderFUZZ})
				ct.PlaceholderCount[PlaceholderFUZZ]++
				i += 4
				literalStart = i
				continue
			}
			if strings.HasPrefix(input[i:], "FOO") {
				if i > literalStart {
					text := input[literalStart:i]
					ct.Segments = append(ct.Segments, GenTemplateSegment{Kind: SegmentLiteral, Text: text})
					ct.LiteralBytes += len(text)
				}
				ct.Segments = append(ct.Segments, GenTemplateSegment{Kind: SegmentPlaceholder, ID: PlaceholderFOO})
				ct.PlaceholderCount[PlaceholderFOO]++
				i += 3
				literalStart = i
				continue
			}
		} else if input[i] == 'B' {
			if strings.HasPrefix(input[i:], "BAR") {
				if i > literalStart {
					text := input[literalStart:i]
					ct.Segments = append(ct.Segments, GenTemplateSegment{Kind: SegmentLiteral, Text: text})
					ct.LiteralBytes += len(text)
				}
				ct.Segments = append(ct.Segments, GenTemplateSegment{Kind: SegmentPlaceholder, ID: PlaceholderBAR})
				ct.PlaceholderCount[PlaceholderBAR]++
				i += 3
				literalStart = i
				continue
			}
			if strings.HasPrefix(input[i:], "BAZ") {
				if i > literalStart {
					text := input[literalStart:i]
					ct.Segments = append(ct.Segments, GenTemplateSegment{Kind: SegmentLiteral, Text: text})
					ct.LiteralBytes += len(text)
				}
				ct.Segments = append(ct.Segments, GenTemplateSegment{Kind: SegmentPlaceholder, ID: PlaceholderBAZ})
				ct.PlaceholderCount[PlaceholderBAZ]++
				i += 3
				literalStart = i
				continue
			}
			if strings.HasPrefix(input[i:], "BUZZ") {
				if i > literalStart {
					text := input[literalStart:i]
					ct.Segments = append(ct.Segments, GenTemplateSegment{Kind: SegmentLiteral, Text: text})
					ct.LiteralBytes += len(text)
				}
				ct.Segments = append(ct.Segments, GenTemplateSegment{Kind: SegmentPlaceholder, ID: PlaceholderBUZZ})
				ct.PlaceholderCount[PlaceholderBUZZ]++
				i += 4
				literalStart = i
				continue
			}
		}
		i++
	}

	if literalStart < len(input) {
		text := input[literalStart:]
		ct.Segments = append(ct.Segments, GenTemplateSegment{Kind: SegmentLiteral, Text: text})
		ct.LiteralBytes += len(text)
	}

	return ct
}

// Render executes the compiled template substituting placeholders with values.
func (t *GenCompiledTemplate) Render(values [5]string) string {
	if len(t.Segments) == 0 {
		return ""
	}
	// Zero-cost fast paths
	if len(t.Segments) == 1 {
		seg := t.Segments[0]
		if seg.Kind == SegmentLiteral {
			return seg.Text
		}
		return values[seg.ID]
	}

	size := t.LiteralBytes +
		int(t.PlaceholderCount[PlaceholderFUZZ])*len(values[PlaceholderFUZZ]) +
		int(t.PlaceholderCount[PlaceholderFOO])*len(values[PlaceholderFOO]) +
		int(t.PlaceholderCount[PlaceholderBAR])*len(values[PlaceholderBAR]) +
		int(t.PlaceholderCount[PlaceholderBAZ])*len(values[PlaceholderBAZ]) +
		int(t.PlaceholderCount[PlaceholderBUZZ])*len(values[PlaceholderBUZZ])

	var b strings.Builder
	b.Grow(size)

	for _, seg := range t.Segments {
		if seg.Kind == SegmentLiteral {
			b.WriteString(seg.Text)
		} else {
			b.WriteString(values[seg.ID])
		}
	}
	return b.String()
}
