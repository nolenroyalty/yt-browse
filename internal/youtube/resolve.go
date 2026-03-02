package youtube

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// ResolveChannel takes a handle (@mkbhd), URL, or channel ID and returns channel info.
func (c *Client) ResolveChannel(ctx context.Context, input string) (*Channel, error) {
	input = strings.TrimSpace(input)

	// Try to parse as URL first
	if strings.Contains(input, "youtube.com") || strings.Contains(input, "youtu.be") {
		return c.resolveFromURL(ctx, input)
	}

	// If starts with @, it's a handle
	if strings.HasPrefix(input, "@") {
		return c.resolveByHandle(ctx, input)
	}

	// If starts with UC, it's a channel ID
	if strings.HasPrefix(input, "UC") && len(input) == 24 {
		return c.resolveByID(ctx, input)
	}

	// Try as handle (user might have omitted @)
	return c.resolveByHandle(ctx, "@"+input)
}

func (c *Client) resolveFromURL(ctx context.Context, rawURL string) (*Channel, error) {
	if !strings.HasPrefix(rawURL, "http") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parsing URL %q: %w", rawURL, err)
	}

	path := strings.TrimSuffix(u.Path, "/")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	if len(parts) == 0 {
		return nil, fmt.Errorf("could not extract channel from URL %q", rawURL)
	}

	switch {
	case parts[0] == "channel" && len(parts) >= 2:
		return c.resolveByID(ctx, parts[1])
	case strings.HasPrefix(parts[0], "@"):
		return c.resolveByHandle(ctx, parts[0])
	case parts[0] == "c" && len(parts) >= 2:
		// Legacy custom URL format /c/Name — try as handle
		return c.resolveByHandle(ctx, "@"+parts[1])
	case parts[0] == "user" && len(parts) >= 2:
		return c.resolveByUsername(ctx, parts[1])
	default:
		// Could be youtube.com/CustomName — try as handle
		return c.resolveByHandle(ctx, "@"+parts[0])
	}
}

func (c *Client) resolveByHandle(ctx context.Context, handle string) (*Channel, error) {
	call := c.service.Channels.List([]string{"snippet", "contentDetails"}).
		ForHandle(handle).
		Context(ctx)

	resp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("channels.list for handle %q: %w", handle, err)
	}
	if len(resp.Items) == 0 {
		return nil, fmt.Errorf("channel not found for handle %q", handle)
	}
	return channelFromItem(resp.Items[0]), nil
}

func (c *Client) resolveByID(ctx context.Context, id string) (*Channel, error) {
	call := c.service.Channels.List([]string{"snippet", "contentDetails"}).
		Id(id).
		Context(ctx)

	resp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("channels.list for id %q: %w", id, err)
	}
	if len(resp.Items) == 0 {
		return nil, fmt.Errorf("channel not found for id %q", id)
	}
	return channelFromItem(resp.Items[0]), nil
}

func (c *Client) resolveByUsername(ctx context.Context, username string) (*Channel, error) {
	call := c.service.Channels.List([]string{"snippet", "contentDetails"}).
		ForUsername(username).
		Context(ctx)

	resp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("channels.list for username %q: %w", username, err)
	}
	if len(resp.Items) == 0 {
		return nil, fmt.Errorf("channel not found for username %q", username)
	}
	return channelFromItem(resp.Items[0]), nil
}

func channelFromItem(item *ytChannel) *Channel {
	ch := &Channel{
		ID:    item.Id,
		Title: item.Snippet.Title,
	}
	if item.Snippet.CustomUrl != "" {
		ch.Handle = item.Snippet.CustomUrl
	}
	if item.Snippet.Description != "" {
		ch.Description = item.Snippet.Description
	}
	if item.ContentDetails != nil && item.ContentDetails.RelatedPlaylists != nil {
		ch.UploadsPlaylistID = item.ContentDetails.RelatedPlaylists.Uploads
	}
	return ch
}
