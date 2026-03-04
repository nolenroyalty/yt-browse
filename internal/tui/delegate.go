package tui

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// filterState holds shared filter state between the Model and the delegate.
type filterState struct {
	text       string
	mode       filterMode
	compiledRe *regexp.Regexp // pre-compiled regex (nil when mode != filterRegex or invalid)
	regexError bool           // true when regex failed to compile (show indicator)
	flashIndex int            // index of item to flash green (-1 = none)
	flashOn    bool           // whether the flash is currently active
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
	var titleMatches []int
	if d.filter.text != "" {
		titleMatches = computeMatches(title, d.filter.text, d.filter)
	}

	isFlashed := d.filter.flashOn && d.filter.flashIndex == index

	if isFlashed {
		greenTitle := s.NormalTitle.Foreground(flashColor)
		greenDesc := s.NormalDesc.Foreground(flashColor)
		title = greenTitle.Render(title)
		desc = greenDesc.Render(desc)
	} else if isSelected {
		if len(titleMatches) > 0 {
			unmatched := s.SelectedTitle.Inline(true)
			matched := unmatched.Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, titleMatches, matched, unmatched)
		}
		title = s.SelectedTitle.Render(title)
		desc = s.SelectedDesc.Render(desc)
	} else {
		if len(titleMatches) > 0 {
			unmatched := s.NormalTitle.Inline(true)
			matched := unmatched.Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, titleMatches, matched, unmatched)
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

// computeMatches returns rune indices in title that match the query.
func computeMatches(title, query string, filter *filterState) []int {
	return matchIndices(title, query, filter.mode, filter.compiledRe)
}
