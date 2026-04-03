# ytui

A terminal-based YouTube client built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea). Browse subscriptions, search videos, watch with mpv, download with yt-dlp, view comments, and see thumbnails rendered in your terminal. Includes a YouTube Music mode for browsing and playing music.

## Features

### Video Mode
- **Search** YouTube videos with infinite scroll
- **Subscription feed** and **channel list** (requires authentication)
- **Video details** with views, likes, description, and thumbnail
- **Comments** with threaded replies, expand/collapse
- **Play** videos with mpv/vlc (background, TUI stays active)
- **Download** videos with yt-dlp
- **Thumbnails** rendered in terminal via Kitty graphics protocol
- **Vim-like keybindings** throughout

### Music Mode
- **Search** YouTube Music (songs, albums, artists, playlists)
- **Home feed** with personalized shelves (requires authentication)
- **Library** with sub-tabs: Playlists, Songs, Albums, Subscriptions (requires authentication)
- **Artist pages** with sub-tabs for songs, albums, singles, videos
- **Album/playlist pages** with track listings
- **Play** songs and albums with mpv
- **"See all" / Load more** for artist sections and library pagination
- **Multi-tab** interface — open multiple artists/albums simultaneously

## Requirements

- [Go](https://go.dev/) 1.21+
- [Kitty](https://sw.kovidgoyal.net/kitty/) terminal (for thumbnail rendering)
- [mpv](https://mpv.io/) (for video playback)
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) (for downloads)

## Install

```sh
go install github.com/deathmaz/ytui/cmd/ytui@latest
```

Or build from source:

```sh
git clone https://github.com/deathmaz/ytui.git
cd ytui
go build ./cmd/ytui
```

## Usage

```sh
ytui                        # start in video mode
ytui -search "go tutorial"  # search immediately on startup
ytui -music                 # start in YouTube Music mode
ytui -music -search "metallica"  # music mode with search
ytui -open "https://www.youtube.com/watch?v=ID"  # open a video directly
ytui -open "https://youtu.be/ID"                 # short URL works too
ytui -open "dQw4w9WgXcQ"                         # raw video ID
ytui -music -open "https://youtube.com/playlist?list=PLxxx"  # open playlist in music mode
```

## Keybindings

### Video Mode Keybindings

| Key | Action |
|-----|--------|
| `1` / `2` / `3` | Switch to Feed / Subs / Search |
| `j` / `k` | Navigate down / up |
| `Ctrl+d` / `Ctrl+u` | Half page down / up |
| `g` / `G` | Top / bottom |
| `/` | Focus search input |
| `Esc` | Go back / blur input |
| `Enter` | Select / search |
| `i` | Video details |
| `p` | Play video (default/config quality) |
| `P` | Play video (pick quality from list) |
| `d` | Download video |
| `Tab` / `Shift+Tab` | Switch between Info / Comments (in detail view) |
| `l` / `h` | Expand / collapse replies (in Comments tab) |
| `L` | Load more (comments / search results) |
| `a` | Authenticate (extract browser cookies) |
| `o` | Open video in browser |
| `O` | Open URL dialog (paste YouTube URL) |
| `y` | Copy video URL to clipboard |
| `r` | Refresh current view |
| `?` | Toggle help |
| `q` | Quit |
| `Ctrl+c` | Force quit |

### Music Mode Keybindings

| Key | Action |
|-----|--------|
| `1` / `2` / `3` | Switch to Home / Library / Search |
| `4`-`9` | Switch to open artist/album tabs |
| `j` / `k` | Navigate down / up |
| `g` / `G` | Top / bottom |
| `/` | Focus search input |
| `Tab` / `Shift+Tab` | Next / previous sub-tab (shelves, sections) |
| `Enter` | Open item (artist/album/playlist) or play song |
| `p` | Play selected item |
| `P` | Play full album/playlist (in album tab) |
| `L` | Load more (artist sections, library pagination) |
| `a` | Authenticate (extract browser cookies) |
| `O` | Open URL dialog (paste YouTube URL) |
| `Esc` | Close current tab / blur input |
| `q` | Quit |

## Configuration

Config file: `~/.config/ytui/config.toml` (respects `$XDG_CONFIG_HOME`)

All settings are optional -- defaults are used for any missing values.

```toml
[player.video]
command = "mpv"                    # player command (default: "mpv")
args = ["--no-terminal"]           # arguments passed to the player
quality = ""                       # default quality: "1080", "720", "480", "best", "audio" (empty = system default)

[player.music]
# command = "mpv"                  # defaults to player.video.command if omitted
# args = ["--no-terminal", "--profile=music"]  # defaults to player.video.args if omitted

[download]
command = "yt-dlp"                 # download command (default: "yt-dlp")
output_dir = "~/Videos/ytui"       # download directory (default: "~/Videos/ytui")
format = ""                        # default yt-dlp format string

[auth]
browser = "brave"                  # browser: brave, chrome, chromium, firefox, edge
auth_on_startup = false            # auto-authenticate on launch (default: false)

[general]
mode = "video"                     # "video" (default) or "music" for YouTube Music mode
```

## Authentication

ytui extracts YouTube session cookies from your browser to access subscriptions and feed. Cookies are held in memory only -- never written to disk.

Supported browsers: **Brave**, **Chrome**, **Chromium**, **Firefox**, **Edge**. Configure which one in the `[auth]` section of the config file.

- Press `a` to authenticate manually
- Set `auth_on_startup = true` in config for automatic authentication

## License

MIT
