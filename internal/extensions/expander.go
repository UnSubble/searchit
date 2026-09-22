package extensions

// Expander encapsulates candidate variant generation and traversal level construction.
type Expander struct {
	Extensions []string
}

// NewExpander creates a new Expander with normalized extensions.
func NewExpander(exts []string) Expander {
	return Expander{Extensions: exts}
}

// HasExtensions reports whether any extensions are configured.
func (e Expander) HasExtensions() bool {
	return len(e.Extensions) > 0
}

// VariantCount returns the number of variants produced per base word.
func (e Expander) VariantCount() int {
	if len(e.Extensions) == 0 {
		return 1
	}
	return len(e.Extensions)
}

// Variants returns all variants for a base word in configured extension order.
func (e Expander) Variants(baseWord string) []string {
	return GenerateVariants(baseWord, e.Extensions)
}

// VariantAt returns the variant at variantIndex for baseWord.
func (e Expander) VariantAt(baseWord string, variantIndex int) string {
	vars := e.Variants(baseWord)
	if variantIndex >= 0 && variantIndex < len(vars) {
		return vars[variantIndex]
	}
	return baseWord
}

// Level returns all candidates at a specific variant index across all base words.
func (e Expander) Level(baseWords []string, levelIndex int) []string {
	res := make([]string, len(baseWords))
	for i, w := range baseWords {
		res[i] = e.VariantAt(w, levelIndex)
	}
	return res
}

// Levels returns slices of candidate words grouped by extension level (for BFS).
func (e Expander) Levels(baseWords []string) [][]string {
	count := e.VariantCount()
	levels := make([][]string, count)
	for i := 0; i < count; i++ {
		levels[i] = e.Level(baseWords, i)
	}
	return levels
}

// ExpandEager produces candidates in eager order: all variants of word 0, then word 1, etc.
func (e Expander) ExpandEager(baseWords []string) []string {
	total := len(baseWords) * e.VariantCount()
	res := make([]string, 0, total)
	for _, w := range baseWords {
		res = append(res, e.Variants(w)...)
	}
	return res
}
