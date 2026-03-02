package tui

import (
	"context"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
	"github.com/nroyalty/yt-browse/internal/cache"
	"github.com/nroyalty/yt-browse/internal/youtube"
)

func resolveChannelCmd(client *youtube.Client, store *cache.Store, input string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		ch, err := client.ResolveChannel(ctx, input)
		if err != nil {
			return channelErrorMsg{err: err}
		}
		// Cache the channel
		_ = store.SetChannel(ch)
		return channelResolvedMsg{channel: ch}
	}
}

func fetchPlaylistsCmd(client *youtube.Client, store *cache.Store, channelID string) tea.Cmd {
	return func() tea.Msg {
		// Check cache first
		if cached, _ := store.GetPlaylists(channelID); cached != nil {
			return playlistsFetchedMsg{playlists: cached}
		}

		ctx := context.Background()
		playlists, err := client.FetchPlaylists(ctx, channelID)
		if err != nil {
			return playlistsErrorMsg{err: err}
		}

		_ = store.SetPlaylists(channelID, playlists)
		return playlistsFetchedMsg{playlists: playlists}
	}
}

func fetchVideosCmd(client *youtube.Client, store *cache.Store, uploadsPlaylistID, channelID string) tea.Cmd {
	return func() tea.Msg {
		// Check cache first
		if cached, _ := store.GetVideos(channelID); cached != nil {
			return videosFetchedMsg{videos: cached}
		}

		ctx := context.Background()
		videos, err := client.FetchVideos(ctx, uploadsPlaylistID, channelID)
		if err != nil {
			return videosErrorMsg{err: err}
		}
		_ = store.SetVideos(channelID, videos)
		return videosFetchedMsg{videos: videos}
	}
}

func fetchPlaylistVideosCmd(client *youtube.Client, store *cache.Store, playlistID, channelID string) tea.Cmd {
	return func() tea.Msg {
		if cached, _ := store.GetPlaylistVideos(channelID, playlistID); cached != nil {
			return playlistVideosFetchedMsg{videos: cached}
		}

		ctx := context.Background()
		videos, err := client.FetchVideos(ctx, playlistID, channelID)
		if err != nil {
			return playlistVideosErrorMsg{err: err}
		}
		_ = store.SetPlaylistVideos(channelID, playlistID, videos)
		return playlistVideosFetchedMsg{videos: videos}
	}
}

func openURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		case "linux":
			cmd = exec.Command("xdg-open", url)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		_ = cmd.Start()
		return nil
	}
}
