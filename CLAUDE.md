# yt-browse

A terminal UI for browsing YouTube channel playlists and videos. Built because YouTube's channel search is terrible -- you can't search playlists by name, can't order video results by date/views, can't do exact string matching, etc. This fixes all of that.

Target audience is primarily the author, but will be open-sourced eventually.

## Quick reference

- **Build & install**: `go build -o yt-browse ./cmd/yt-browse && mv yt-browse ~/bin/` (or `./install.sh`)
- **Run**: `yt-browse <channel>` -- accepts @handle, channel URL, channel ID, or bare username
- **Required env**: `YT_BROWSE_API_KEY` (YouTube Data API v3 key)
- **Optional env**: `YT_BROWSE_CACHE_DIR` (default `~/.yt-browse/cache`), `YT_BROWSE_CACHE_TTL` (default `24h`)

## Architecture

Standard Bubble Tea (Elm architecture) app. Unidirectional data flow: Cmds fetch data async, Msgs deliver results, Update applies state changes, View renders.

```
cmd/yt-browse/main.go        -- entry point, arg parsing
internal/
  config/                     -- env var config (API key, cache settings)
  cache/                      -- file-based JSON cache with TTL, generic over data type
  youtube/
    types.go                  -- Channel, Playlist, Video domain types
    client.go                 -- YouTube API v3 wrapper (playlists, videos)
    resolve.go                -- flexible channel resolution (handle, URL, ID)
  tui/
    app.go                    -- main Model, Update loop, View (the big file)
    commands.go               -- Bubble Tea Cmds (check cache, then API, then write cache)
    messages.go               -- Msg types returned by Cmds
    keys.go                   -- keybinding definitions
    styles.go                 -- lipgloss styles
    delegate.go               -- custom list delegate with fuzzy match highlighting
    detail.go                 -- right-side detail pane rendering
    playlist_item.go          -- list.DefaultItem adapter for Playlist
    video_item.go             -- list.DefaultItem adapter for Video
```

## Key design decisions

- **Three separate list.Model instances** (playlists, videos, playlist-videos) so scroll position / selection is preserved when switching tabs.
- **Raw data kept separate from list items** -- canonical data in `[]youtube.Playlist` / `[]youtube.Video` slices, list items recomputed on every sort/filter via `applyFilterAndSort()`.
- **Custom filter system** instead of Bubble Tea's built-in list filtering. Supports both fuzzy (default, via `sahilm/fuzzy`) and exact (case-insensitive substring) modes. Filter searches both title and description.
- **Shared mutable `*filterState` pointer** between Model and delegate -- intentional shortcut that works because Bubble Tea is single-threaded.
- **Fuzzy relevance vs explicit sort**: when no sort is active, fuzzy results rank by relevance. When a sort IS active, it wins over fuzzy relevance.
- **Videos fetched eagerly** in the background as soon as playlists load, so switching to the Videos tab is instant.
- **Shorts hidden by default** (videos <= 60s). Toggle with `s`.

## YouTube API usage

- Uses API key auth (read-only public data), not OAuth.
- Avoids the YouTube Search API (expensive quota). Instead lists all videos/playlists for a channel (cheap).
- Two-step video fetch: `PlaylistItems.List` for IDs, then `Videos.List` for stats/duration (API limitation).
- Has been tested on channels with 20k+ videos.
- Attempted to get "last video added" date for playlists (for sorting) but the API doesn't support it.

## Caching

- File-based JSON under `~/.yt-browse/cache/<channelID>/`.
- 24h TTL by default. Users can force-refresh with `r`.
- Cache write errors are silently ignored -- app works fine without cache.

## Known context

- Sorting + filtering interaction has been the trickiest area. Current behavior: sort and filter compose correctly, with explicit sort overriding fuzzy relevance ranking.
- The Esc key is overloaded: clears filter when active, goes back from drill-in view when no filter.
