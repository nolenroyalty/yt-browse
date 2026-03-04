package tui

import (
	"github.com/nroyalty/yt-browse/internal/recent"
	"github.com/nroyalty/yt-browse/internal/youtube"
)

// Channel resolution
type channelResolvedMsg struct{ channel *youtube.Channel }
type channelErrorMsg struct{ err error }

// Playlist fetching
type playlistsFetchedMsg struct{ playlists []youtube.Playlist }
type playlistsErrorMsg struct{ err error }

// Video fetching (uploads playlist)
type videosFetchedMsg struct {
	videos []youtube.Video
	gen    int
}
type videosErrorMsg struct {
	err error
	gen int
}

// Video loading progress
type videoLoadingMsg struct {
	total  int
	loaded int
	gen    int
}

// Playlist video fetching (drill-in)
type playlistVideosFetchedMsg struct{ videos []youtube.Video }
type playlistVideosErrorMsg struct{ err error }

// Recent channels
type recentChannelsLoadedMsg struct{ entries []recent.Entry }
type recentChannelRemovedMsg struct{}

// UI flash
type clearFlashMsg struct{}

// Filter debounce
type filterDebounceMsg struct{ seq int }
