package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
)

func renderDetail(item list.Item, width int) string {
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
		if p.Description != "" {
			b.WriteString("\n")
			b.WriteString(detailLabelStyle.Render("Description:"))
			b.WriteString("\n")
			b.WriteString(detailValueStyle.Render(wrap(p.Description)))
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
		if vid.Description != "" {
			b.WriteString("\n")
			b.WriteString(detailLabelStyle.Render("Description:"))
			b.WriteString("\n")
			b.WriteString(detailValueStyle.Render(wrap(vid.Description)))
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
