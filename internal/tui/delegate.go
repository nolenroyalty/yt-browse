package tui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
)

// filterState holds shared filter state between the Model and the delegate.
type filterState struct {
	text string
	mode filterMode
}

// highlightDelegate wraps list.DefaultDelegate to add match highlighting
// when our custom filter is active.
type highlightDelegate struct {
	list.DefaultDelegate
	filter *filterState
}

func newHighlightDelegate(filter *filterState) *highlightDelegate {
	return &highlightDelegate{
		DefaultDelegate: list.NewDefaultDelegate(),
		filter:          filter,
	}
}

// Render renders an item, highlighting matched characters when a filter is active.
func (d *highlightDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(list.DefaultItem)
	if !ok {
		return
	}

	title := i.Title()
	desc := i.Description()
	s := &d.Styles

	if m.Width() <= 0 {
		return
	}

	// Prevent text from exceeding list width
	textwidth := m.Width() - s.NormalTitle.GetPaddingLeft() - s.NormalTitle.GetPaddingRight()
	title = ansi.Truncate(title, textwidth, "…")
	if d.ShowDescription {
		var lines []string
		for li, line := range strings.Split(desc, "\n") {
			if li >= d.Height()-1 {
				break
			}
			lines = append(lines, ansi.Truncate(line, textwidth, "…"))
		}
		desc = strings.Join(lines, "\n")
	}

	isSelected := index == m.Index()

	// Compute match indices if filter is active
	var matchedRunes []int
	if d.filter.text != "" {
		matchedRunes = computeTitleMatches(title, d.filter.text, d.filter.mode)
	}

	if isSelected {
		if len(matchedRunes) > 0 {
			unmatched := s.SelectedTitle.Inline(true)
			matched := unmatched.Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}
		title = s.SelectedTitle.Render(title)
		desc = s.SelectedDesc.Render(desc)
	} else {
		if len(matchedRunes) > 0 {
			unmatched := s.NormalTitle.Inline(true)
			matched := unmatched.Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}
		title = s.NormalTitle.Render(title)
		desc = s.NormalDesc.Render(desc)
	}

	if d.ShowDescription {
		fmt.Fprintf(w, "%s\n%s", title, desc)
		return
	}
	fmt.Fprintf(w, "%s", title)
}

// computeTitleMatches returns rune indices in title that match the query.
func computeTitleMatches(title, query string, mode filterMode) []int {
	if mode == filterExact {
		titleLower := strings.ToLower(title)
		queryLower := strings.ToLower(query)
		idx := strings.Index(titleLower, queryLower)
		if idx < 0 {
			return nil
		}
		// Convert byte offset to rune offset
		runeOffset := len([]rune(titleLower[:idx]))
		queryRuneLen := len([]rune(query))
		indices := make([]int, queryRuneLen)
		for i := range indices {
			indices[i] = runeOffset + i
		}
		return indices
	}

	// Fuzzy match
	matches := fuzzy.Find(query, []string{title})
	if len(matches) == 0 {
		return nil
	}
	return matches[0].MatchedIndexes
}
