package youtube

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sosodev/duration"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	yt "google.golang.org/api/youtube/v3"
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
// If onProgress is non-nil, it is called with (total, loaded) counts during fetching.
//
// This is unavoidably slow for large channels. The YouTube API requires two steps:
// (1) PlaylistItems.List to get video IDs, (2) Videos.List for details (stats, duration).
// Step 1 uses opaque cursor-based pagination (nextPageToken) with max 50 items per page,
// so there's no way to parallelize it — each page requires the previous page's token.
// The Search API supports date-range filters that would allow partitioning, but costs
// 100 quota units/request vs 1 for PlaylistItems, making it infeasible for large channels.
//
// We do the best we can: detail fetches are pipelined behind the pagination loop so that
// as each page of IDs arrives, a goroutine immediately fetches details (up to 5 concurrent).
// The Go HTTP client reuses connections (HTTP/2) to googleapis.com automatically.
func (c *Client) FetchVideos(ctx context.Context, playlistID string, channelID string, onProgress func(total, loaded int)) ([]Video, error) {
	var (
		mu        sync.Mutex
		allVideos []Video
		fetchErr  error
		wg        sync.WaitGroup
	)

	sem := make(chan struct{}, 5) // max 5 concurrent detail requests
	total := 0
	pageToken := ""

	for {
		call := c.service.PlaylistItems.List([]string{"contentDetails"}).
			PlaylistId(playlistID).
			MaxResults(50).
			Fields(googleapi.Field("nextPageToken,pageInfo/totalResults,items/contentDetails/videoId")).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			wg.Wait()
			return nil, fmt.Errorf("playlistItems.list for playlist %s: %w", playlistID, err)
		}

		if total == 0 && resp.PageInfo != nil {
			total = int(resp.PageInfo.TotalResults)
			if onProgress != nil {
				onProgress(total, 0)
			}
		}

		var ids []string
		for _, item := range resp.Items {
			if item.ContentDetails != nil {
				ids = append(ids, item.ContentDetails.VideoId)
			}
		}

		if len(ids) > 0 {
			wg.Add(1)
			go func(ids []string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				videos, err := c.fetchVideoDetails(ctx, ids, channelID)

				var loaded int
				mu.Lock()
				if err != nil {
					if fetchErr == nil {
						fetchErr = err
					}
					mu.Unlock()
					return
				}
				allVideos = append(allVideos, videos...)
				loaded = len(allVideos)
				mu.Unlock()

				if onProgress != nil {
					onProgress(total, loaded)
				}
			}(ids)
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	wg.Wait()

	if fetchErr != nil {
		return nil, fetchErr
	}
	return allVideos, nil
}

// fetchVideoDetails gets duration, views, likes for a batch of video IDs (max 50).
func (c *Client) fetchVideoDetails(ctx context.Context, videoIDs []string, channelID string) ([]Video, error) {
	call := c.service.Videos.List([]string{"snippet", "contentDetails", "statistics"}).
		Id(videoIDs...).
		Fields(googleapi.Field("items(id,snippet(title,description,publishedAt,thumbnails/medium/url),contentDetails/duration,statistics(viewCount,likeCount))")).
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
