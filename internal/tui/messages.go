package tui

import "github.com/nroyalty/yt-browse/internal/youtube"

// Channel resolution
type channelResolvedMsg struct{ channel *youtube.Channel }
type channelErrorMsg struct{ err error }

// Playlist fetching
type playlistsFetchedMsg struct{ playlists []youtube.Playlist }
type playlistsErrorMsg struct{ err error }

// Video fetching (uploads playlist)
type videosFetchedMsg struct{ videos []youtube.Video }
type videosErrorMsg struct{ err error }

// Playlist video fetching (drill-in)
type playlistVideosFetchedMsg struct{ videos []youtube.Video }
type playlistVideosErrorMsg struct{ err error }
