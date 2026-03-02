package youtube

import (
	"context"
	"fmt"
	"time"

	"github.com/sosodev/duration"
	yt "google.golang.org/api/youtube/v3"
	"google.golang.org/api/option"
)

// Type alias to avoid collision with our Channel type in resolve.go
type ytChannel = yt.Channel

type Client struct {
	service *yt.Service
}

func NewClient(ctx context.Context, apiKey string) (*Client, error) {
	svc, err := yt.NewService(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("creating youtube service: %w", err)
	}
	return &Client{service: svc}, nil
}

// FetchPlaylists returns all playlists for a channel, paginated.
func (c *Client) FetchPlaylists(ctx context.Context, channelID string) ([]Playlist, error) {
	var playlists []Playlist
	pageToken := ""

	for {
		call := c.service.Playlists.List([]string{"snippet", "contentDetails"}).
			ChannelId(channelID).
			MaxResults(50).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("playlists.list for channel %s: %w", channelID, err)
		}

		for _, item := range resp.Items {
			pub, _ := time.Parse(time.RFC3339, item.Snippet.PublishedAt)
			p := Playlist{
				ID:          item.Id,
				ChannelID:   channelID,
				Title:       item.Snippet.Title,
				Description: item.Snippet.Description,
				PublishedAt: pub,
				ItemCount:   item.ContentDetails.ItemCount,
			}
			if item.Snippet.Thumbnails != nil && item.Snippet.Thumbnails.Medium != nil {
				p.ThumbnailURL = item.Snippet.Thumbnails.Medium.Url
			}
			playlists = append(playlists, p)
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return playlists, nil
}

// FetchVideos returns all videos from a playlist with full details.
// Works for uploads playlists, user-created playlists, or any other playlist ID.
func (c *Client) FetchVideos(ctx context.Context, playlistID string, channelID string) ([]Video, error) {
	var allVideos []Video
	pageToken := ""

	for {
		call := c.service.PlaylistItems.List([]string{"snippet", "contentDetails"}).
			PlaylistId(playlistID).
			MaxResults(50).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("playlistItems.list for playlist %s: %w", playlistID, err)
		}

		// Collect video IDs for this page
		var videoIDs []string
		for _, item := range resp.Items {
			if item.ContentDetails != nil {
				videoIDs = append(videoIDs, item.ContentDetails.VideoId)
			}
		}

		// Batch-fetch video details
		if len(videoIDs) > 0 {
			details, err := c.fetchVideoDetails(ctx, videoIDs, channelID)
			if err != nil {
				return nil, err
			}
			allVideos = append(allVideos, details...)
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return allVideos, nil
}

// fetchVideoDetails gets duration, views, likes for a batch of video IDs (max 50).
func (c *Client) fetchVideoDetails(ctx context.Context, videoIDs []string, channelID string) ([]Video, error) {
	call := c.service.Videos.List([]string{"snippet", "contentDetails", "statistics"}).
		Id(videoIDs...).
		Context(ctx)

	resp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("videos.list: %w", err)
	}

	videos := make([]Video, 0, len(resp.Items))
	for _, item := range resp.Items {
		v := Video{
			ID:          item.Id,
			ChannelID:   channelID,
			Title:       item.Snippet.Title,
			Description: item.Snippet.Description,
		}

		if pub, err := time.Parse(time.RFC3339, item.Snippet.PublishedAt); err == nil {
			v.PublishedAt = pub
		}

		if item.Snippet.Thumbnails != nil && item.Snippet.Thumbnails.Medium != nil {
			v.ThumbnailURL = item.Snippet.Thumbnails.Medium.Url
		}

		if item.ContentDetails != nil {
			v.Duration = parseDuration(item.ContentDetails.Duration)
		}

		if item.Statistics != nil {
			v.ViewCount = item.Statistics.ViewCount
			v.LikeCount = item.Statistics.LikeCount
		}

		videos = append(videos, v)
	}

	return videos, nil
}

// parseDuration converts ISO 8601 duration (PT15M33S) to time.Duration.
func parseDuration(iso string) time.Duration {
	if iso == "" {
		return 0
	}
	d, err := duration.Parse(iso)
	if err != nil {
		return 0
	}
	return d.ToTimeDuration()
}
