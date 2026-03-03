# YouTube Data API v3 - Reference for yt-browse

Condensed reference covering only the resources and fields relevant to this project.
All methods cost **1 quota unit** per call. We use API key auth (read-only, public data).

---

## Channels

### channels.list

```
GET https://www.googleapis.com/youtube/v3/channels
```

**Filter parameters** (exactly one required):

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Comma-separated channel IDs |
| `forHandle` | string | YouTube handle (with or without `@`) |
| `forUsername` | string | Legacy YouTube username |
| `mine` | boolean | Authenticated user's channel (requires OAuth) |

**Optional**: `part` (required, see below), `hl`, `maxResults` (0-50, default 5), `pageToken`

### Channel resource

**Parts we request**: `snippet`, `contentDetails`

```
snippet.title                    string     Channel name
snippet.description              string     Channel description (max 1000 chars)
snippet.customUrl                string     Custom URL slug
snippet.publishedAt              datetime   ISO 8601 creation date
snippet.thumbnails               object     Keys: default, medium, high
snippet.country                  string     Country code

contentDetails.relatedPlaylists.uploads   string   ← THE uploads playlist ID (UU...)
contentDetails.relatedPlaylists.likes     string   Liked videos playlist ID

statistics.viewCount             unsigned long   Total views across all videos
statistics.subscriberCount       unsigned long   Subscriber count (rounded to 3 sig figs)
statistics.videoCount            unsigned long   Public video count
statistics.hiddenSubscriberCount boolean         Whether sub count is hidden

status.privacyStatus             string     private | public | unlisted
status.madeForKids               boolean
```

**What we use**: `snippet.title`, `snippet.customUrl` (as handle), `snippet.description`, `contentDetails.relatedPlaylists.uploads`

---

## Playlists

### playlists.list

```
GET https://www.googleapis.com/youtube/v3/playlists
```

**Filter parameters** (exactly one required):

| Parameter | Type | Description |
|-----------|------|-------------|
| `channelId` | string | Return playlists for this channel |
| `id` | string | Comma-separated playlist IDs |
| `mine` | boolean | Authenticated user's playlists (requires OAuth) |

**Optional**: `part`, `hl`, `maxResults` (0-50, default 5), `pageToken`

### Playlist resource

**Parts we request**: `snippet`, `contentDetails`

```
id                               string     Playlist ID
snippet.publishedAt              datetime   ISO 8601 creation date
snippet.channelId                string     Owner channel ID
snippet.title                    string     Playlist title
snippet.description              string     Playlist description
snippet.thumbnails               object     Keys: default (120x90), medium (320x180),
                                            high (480x360), standard (640x480), maxres (1280x720)
snippet.channelTitle             string     Owner channel name
snippet.defaultLanguage          string     Metadata language

contentDetails.itemCount         unsigned int   Number of videos in playlist

status.privacyStatus             string     private | public | unlisted
status.podcastStatus             enum       enabled | disabled | unspecified
```

**What we use**: `id`, `snippet.title`, `snippet.description`, `snippet.publishedAt`, `snippet.thumbnails.medium`, `contentDetails.itemCount`

**Notable absence**: No field for "last video added" date. `publishedAt` is playlist *creation* date only.

---

## PlaylistItems

### playlistItems.list

```
GET https://www.googleapis.com/youtube/v3/playlistItems
```

**Filter parameters** (exactly one required):

| Parameter | Type | Description |
|-----------|------|-------------|
| `playlistId` | string | Return items from this playlist |
| `id` | string | Comma-separated playlist item IDs |

**Optional**: `part`, `maxResults` (0-50, default 5), `pageToken`, `videoId` (filter to specific video)

### PlaylistItem resource

**Parts we request**: `snippet`, `contentDetails`

```
id                               string     Playlist item ID (not the video ID)
snippet.publishedAt              datetime   When item was ADDED to playlist (not video upload date)
snippet.title                    string     Video title
snippet.description              string     Video description
snippet.thumbnails               object     Same keys as playlist thumbnails
snippet.channelTitle             string     Playlist owner channel name
snippet.videoOwnerChannelTitle   string     Video uploader channel name
snippet.videoOwnerChannelId      string     Video uploader channel ID
snippet.playlistId               string     Parent playlist ID
snippet.position                 unsigned int   Zero-based position in playlist
snippet.resourceId.kind          string     Usually "youtube#video"
snippet.resourceId.videoId       string     The actual video ID

contentDetails.videoId           string     Video ID (same as resourceId.videoId)
contentDetails.videoPublishedAt  datetime   Original video publication date
contentDetails.startAt           string     DEPRECATED
contentDetails.endAt             string     DEPRECATED
contentDetails.note              string     User note (max 280 chars)

status.privacyStatus             string     private | public | unlisted
```

**What we use**: `contentDetails.videoId` (to batch-fetch full video details via videos.list)

**Note on "last added" for playlists**: `snippet.publishedAt` here is the date the item was added to the playlist. In theory you could fetch page 1 of a playlist's items sorted by position descending to find the most recently added video, but the API returns items in playlist order (by position) and doesn't support sorting. You'd have to fetch ALL items and find the max `publishedAt`, which is expensive for large playlists.

---

## Videos

### videos.list

```
GET https://www.googleapis.com/youtube/v3/videos
```

**Filter parameters** (exactly one required):

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Comma-separated video IDs (max 50) |
| `chart` | string | `mostPopular` - returns popular videos for region |
| `myRating` | string | `like` or `dislike` (requires OAuth) |

**Optional**: `part`, `hl`, `maxResults` (1-50, default 5), `pageToken`, `regionCode`, `videoCategoryId`

### Video resource

**Parts we request**: `snippet`, `contentDetails`, `statistics`

```
id                               string     Video ID
snippet.publishedAt              datetime   ISO 8601 upload date
snippet.channelId                string     Uploader channel ID
snippet.title                    string     Video title (max 100 chars)
snippet.description              string     Video description (max 5000 bytes)
snippet.thumbnails               object     Keys: default, medium, high, standard, maxres
snippet.channelTitle             string     Uploader channel name
snippet.tags[]                   list       Keywords (max 500 chars total)
snippet.categoryId               string     Video category ID
snippet.liveBroadcastContent     string     none | upcoming | live
snippet.defaultLanguage          string     Metadata language
snippet.defaultAudioLanguage     string     Audio language

contentDetails.duration          string     ISO 8601 duration (e.g. "PT15M33S")
contentDetails.dimension         string     "2d" | "3d"
contentDetails.definition        string     "hd" | "sd"
contentDetails.caption           string     "true" | "false" (as strings, not booleans)
contentDetails.licensedContent   boolean
contentDetails.regionRestriction object     { allowed: [], blocked: [] }
contentDetails.projection        string     "rectangular" | "360"
contentDetails.hasCustomThumbnail boolean

statistics.viewCount             unsigned long
statistics.likeCount             unsigned long
statistics.dislikeCount          unsigned long   (always 0 since dislikes were hidden)
statistics.favoriteCount         unsigned long   (deprecated, always 0)
statistics.commentCount          unsigned long

status.uploadStatus              string     deleted | failed | processed | rejected | uploaded
status.privacyStatus             string     private | public | unlisted
status.embeddable                boolean
status.madeForKids               boolean
status.containsSyntheticMedia    boolean

fileDetails.videoStreams[].widthPixels    unsigned int   ← requires OAuth (owner only)
fileDetails.videoStreams[].heightPixels   unsigned int   ← requires OAuth (owner only)
fileDetails.videoStreams[].aspectRatio    double         ← requires OAuth (owner only)
```

**What we use**: `id`, `snippet.title`, `snippet.description`, `snippet.publishedAt`, `snippet.thumbnails.medium`, `contentDetails.duration`, `statistics.viewCount`, `statistics.likeCount`

**Shorts detection**: No API field indicates whether a video is a Short. We use `duration <= 60s` as a heuristic. The `fileDetails` part (which has aspect ratio) requires OAuth with video owner access, so it's not available with API key auth.

---

## Pagination

All list methods follow the same pattern:
- Response includes `nextPageToken` and `prevPageToken`
- Pass `pageToken` on subsequent requests
- `pageInfo.totalResults` gives the total count
- `maxResults` caps at 50 per page

## Quota

- All list methods: **1 unit per call**
- Default daily quota: **10,000 units**
- We avoid the Search API (`youtube.search.list`) which costs **100 units per call**
- Fetching all videos for a channel with N videos costs roughly: `ceil(N/50)` (playlistItems.list) + `ceil(N/50)` (videos.list) = `ceil(N/25)` units total
- Example: 20,000 videos ≈ 800 quota units
