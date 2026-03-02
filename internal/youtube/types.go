package youtube

import "time"

type Channel struct {
	ID                string
	Title             string
	Handle            string
	Description       string
	UploadsPlaylistID string
}

type Playlist struct {
	ID           string
	ChannelID    string
	Title        string
	Description  string
	PublishedAt  time.Time
	ItemCount    int64
	ThumbnailURL string
}

func (p Playlist) URL() string {
	return "https://www.youtube.com/playlist?list=" + p.ID
}

type Video struct {
	ID           string
	ChannelID    string
	Title        string
	Description  string
	PublishedAt  time.Time
	Duration     time.Duration
	ViewCount    uint64
	LikeCount    uint64
	ThumbnailURL string
}

func (v Video) URL() string {
	return "https://www.youtube.com/watch?v=" + v.ID
}
