package tui

import (
	"regexp"
	"strings"

	"github.com/sahilm/fuzzy"
)

// matchIndices returns rune indices in text that match the query for the given
// filter mode. For regex mode, compiledRe must be non-nil. Returns nil on no match.
func matchIndices(text, query string, mode filterMode, compiledRe *regexp.Regexp) []int {
	switch mode {
	case filterExact:
		titleLower := strings.ToLower(text)
		queryLower := strings.ToLower(query)
		idx := strings.Index(titleLower, queryLower)
		if idx < 0 {
			return nil
		}
		runeOffset := len([]rune(titleLower[:idx]))
		queryRuneLen := len([]rune(query))
		indices := make([]int, queryRuneLen)
		for i := range indices {
			indices[i] = runeOffset + i
		}
		return indices

	case filterWords:
		titleLower := strings.ToLower(text)
		words := strings.Fields(strings.ToLower(query))
		var indices []int
		for _, w := range words {
			idx := strings.Index(titleLower, w)
			if idx < 0 {
				continue
			}
			runeOffset := len([]rune(titleLower[:idx]))
			for i := range len([]rune(w)) {
				indices = append(indices, runeOffset+i)
			}
		}
		return indices

	case filterRegex:
		if compiledRe == nil {
			return nil
		}
		loc := compiledRe.FindStringIndex(text)
		if loc == nil {
			return nil
		}
		runeStart := len([]rune(text[:loc[0]]))
		runeEnd := len([]rune(text[:loc[1]]))
		indices := make([]int, runeEnd-runeStart)
		for i := range indices {
			indices[i] = runeStart + i
		}
		return indices

	default:
		// Fuzzy match (single item)
		matches := fuzzy.Find(query, []string{text})
		if len(matches) == 0 {
			return nil
		}
		return matches[0].MatchedIndexes
	}
}
