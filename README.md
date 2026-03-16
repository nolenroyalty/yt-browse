# yt-browse

`yt-browse` is a TUI that gives you power-user search for a youtube channel's playlists and videos.

https://github.com/user-attachments/assets/2a01ae61-c3bf-474e-b9d9-3da256190327

The youtube channel search experience is *bad*. You can't limit your search to just playlists or videos, filter out shorts, order your results in any way, or restrict your search to a date range.

`yt-browse` fixes that:
* Search a channel's playlists or videos separately
* Regex and fuzzy search
* Limit your search to just titles or include full-text search of descriptions
* Order results by upload date, view count, or video duration
* Search and order videos *within* a playlist
* Restrict your search to specific date ranges with `before:` and `after:` filters
* Copy URLs to your clipboard or open directly in your browser
* All data is cached; search is instant after an initial (once a day per channel) download
* Running without arguments lets you pick from recent channels

`yt-browse` is built for my own needs; my favorite channels have thousands of videos and hundreds of playlists and it's a pain to find the one that I want. But through the magic of vibe-coding, it wasn't that much work to (hopefully) make it usable for you too.

You're welcome to send pull requests to fix bugs. You're also welcome to make feature requests, but since this is primarily a tool for *me* I will close any feature request that I don't want. Feel free to fork :)

## Setup

The Youtube API requires an API key. The API is free; the default limits are relatively generous.

### 1. Get a YouTube API key

1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Create a project (or use an existing one)
3. Enable the **YouTube Data API v3**
4. Create an API key under **Credentials**

Again: the API is free. The default quota (~10,000 units/day) is enough to browse ~500k playlists or ~250k videos per day. Results are cached locally for 24 hours so repeatedly visiting a channel on the same day doesn't cost you API quota.

### 2. Set the environment variable

```bash
export YT_BROWSE_API_KEY=your_key_here
```

Add this to your shell profile (`.bashrc`, `.zshrc`, etc.) to persist it.

### 3. Build

Requires [Go](https://go.dev/) 1.21+.

```bash
git clone https://github.com/ziyunli/yt-browse
cd yt-browse
go build -o yt-browse ./cmd/yt-browse
```

This produces a `yt-browse` binary in the current directory. Run it directly with `./yt-browse`, or move it somewhere on your `PATH`.

There's also an `install.sh` script that builds and copies the binary to `~/bin/`:

```bash
./install.sh
```

## Usage

```
yt-browse [channel]
```

The channel argument accepts any of:
- `@handle` (e.g. `@3blue1brown`)
- Channel URL (e.g. `https://youtube.com/@3blue1brown`)
- Channel ID (e.g. `UCxxxxxxxxxxxxxxxxxxxx`)
- Bare username

If omitted, opens a picker with your recently-browsed channels.

### Environment variables

| Variable              | Default              | Description                    |
| --------------------- | -------------------- | ------------------------------ |
| `YT_BROWSE_API_KEY`   | *(required)*         | YouTube Data API v3 key        |
| `YT_BROWSE_CACHE_DIR` | `~/.yt-browse/cache` | Cache directory                |
| `YT_BROWSE_CACHE_TTL` | `24h`                | Cache TTL (Go duration format) |

## Keybindings

### Navigation

| Key         | Action                              |
| ----------- | ----------------------------------- |
| `tab`       | Switch between playlists / videos   |
| `enter`     | Open video / drill into playlist    |
| `o`         | Open selected item in browser       |
| `y`         | Copy URL to clipboard               |
| `backspace` | Back to playlists (from drill view) |

### Filter & sort

| Key       | Action                                        |
| --------- | --------------------------------------------- |
| `/`       | Start filtering                               |
| `esc`     | Clear filter                                  |
| `ctrl+t`  | Cycle filter mode (words / regex / fuzzy)     |
| `ctrl+d`  | Toggle title-only / title+description search  |
| `d` / `D` | Sort by date (newest / oldest)                |
| `v` / `V` | Sort by views (most / fewest)                 |
| `u` / `U` | Sort by duration (longest / shortest)         |
| `c` / `C` | Sort by count (most / fewest, playlists only) |

You can also use `before:` and `after:` date filters (e.g. `after:2024 before:2025-06`).

### Other

| Key            | Action                               |
| -------------- | ------------------------------------ |
| `r`            | Refresh data from API (bypass cache) |
| `?`            | Show keybindings overlay             |
| `q` / `ctrl+c` | Quit                                 |
