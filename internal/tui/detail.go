package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
)

func renderDetail(item list.Item, width int, filter *filterState) string {
	if item == nil {
		return ""
	}

	var b strings.Builder

	wrap := func(s string) string {
		if width > 0 {
			return lipgloss.Wrap(s, width, "")
		}
		return s
	}

	highlightDesc := func(s string) string {
		wrapped := wrap(s)
		if filter == nil || filter.text == "" || filter.mode == filterFuzzy {
			return detailValueStyle.Render(wrapped)
		}
		// Highlight line by line to preserve newlines
		lines := strings.Split(wrapped, "\n")
		for i, line := range lines {
			matches := computeMatches(line, filter.text, filter.mode)
			if len(matches) > 0 {
				unmatched := detailValueStyle.Inline(true)
				matched := unmatched.Underline(true)
				lines[i] = lipgloss.StyleRunes(line, matches, matched, unmatched)
			} else {
				lines[i] = detailValueStyle.Render(line)
			}
		}
		return strings.Join(lines, "\n")
	}

	switch v := item.(type) {
	case PlaylistItem:
		p := v.playlist
		b.WriteString(detailTitleStyle.Render(wrap(p.Title)))
		b.WriteString("\n\n")
		b.WriteString(detailLabelStyle.Render("Videos: "))
		b.WriteString(detailValueStyle.Render(fmt.Sprintf("%d", p.ItemCount)))
		b.WriteString("\n")
		b.WriteString(detailLabelStyle.Render("Created: "))
		b.WriteString(detailValueStyle.Render(p.PublishedAt.Format("Jan 2, 2006")))
		b.WriteString("\n")
		b.WriteString(detailLabelStyle.Render("URL: "))
		b.WriteString(detailValueStyle.Render(p.URL()))
		b.WriteString("\n")
		b.WriteString("\n")
		b.WriteString(detailLabelStyle.Render("Description:"))
		b.WriteString("\n")
		if p.Description != "" {
			b.WriteString(highlightDesc(p.Description))
		} else {
			b.WriteString(detailLabelStyle.Render("No description"))
		}

	case VideoItem:
		vid := v.video
		b.WriteString(detailTitleStyle.Render(wrap(vid.Title)))
		b.WriteString("\n\n")
		b.WriteString(detailLabelStyle.Render("Duration: "))
		b.WriteString(detailValueStyle.Render(formatDuration(vid.Duration)))
		b.WriteString("\n")
		b.WriteString(detailLabelStyle.Render("Views: "))
		b.WriteString(detailValueStyle.Render(formatCount(vid.ViewCount)))
		b.WriteString("\n")
		b.WriteString(detailLabelStyle.Render("Likes: "))
		b.WriteString(detailValueStyle.Render(formatCount(vid.LikeCount)))
		b.WriteString("\n")
		b.WriteString(detailLabelStyle.Render("Published: "))
		b.WriteString(detailValueStyle.Render(vid.PublishedAt.Format("Jan 2, 2006")))
		b.WriteString("\n")
		b.WriteString(detailLabelStyle.Render("URL: "))
		b.WriteString(detailValueStyle.Render(vid.URL()))
		b.WriteString("\n")
		b.WriteString("\n")
		b.WriteString(detailLabelStyle.Render("Description:"))
		b.WriteString("\n")
		if vid.Description != "" {
			b.WriteString(highlightDesc(vid.Description))
		} else {
			b.WriteString(detailLabelStyle.Render("No description"))
		}
	}

	return b.String()
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "live/unknown"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func formatCount(n uint64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
