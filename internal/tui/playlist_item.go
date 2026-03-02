package tui

import (
	"fmt"

	"github.com/nroyalty/yt-browse/internal/youtube"
)

// PlaylistItem wraps youtube.Playlist and implements list.DefaultItem.
type PlaylistItem struct {
	playlist youtube.Playlist
}

func (p PlaylistItem) Title() string { return p.playlist.Title }
func (p PlaylistItem) Description() string {
	return fmt.Sprintf("%d videos  |  created: %s", p.playlist.ItemCount, p.playlist.PublishedAt.Format("Jan 2, 2006"))
}
func (p PlaylistItem) FilterValue() string { return p.playlist.Title + " " + p.playlist.Description }
func (p PlaylistItem) URL() string          { return p.playlist.URL() }
