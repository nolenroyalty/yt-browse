package tui

import (
	"fmt"

	"github.com/nroyalty/yt-browse/internal/youtube"
)

// VideoItem wraps youtube.Video and implements list.DefaultItem.
type VideoItem struct {
	video youtube.Video
}

func (v VideoItem) Title() string { return v.video.Title }
func (v VideoItem) Description() string {
	return fmt.Sprintf("%s  |  %s views  |  %s",
		formatDuration(v.video.Duration),
		formatCount(v.video.ViewCount),
		v.video.PublishedAt.Format("Jan 2, 2006"),
	)
}
func (v VideoItem) FilterValue() string { return v.video.Title + " " + v.video.Description }
func (v VideoItem) URL() string          { return v.video.URL() }
