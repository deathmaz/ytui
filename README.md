# ytui

A terminal-based YouTube client built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea). Browse subscriptions, search videos, watch with mpv, download with yt-dlp, view comments, and see thumbnails rendered in your terminal.

## Features

- **Search** YouTube videos with infinite scroll
- **Subscription feed** and **channel list** (requires authentication)
- **Video details** with views, likes, description, and thumbnail
- **Comments** with threaded replies, expand/collapse
- **Play** videos with mpv/vlc (background, TUI stays active)
- **Download** videos with yt-dlp
- **Thumbnails** rendered in terminal via Kitty graphics protocol
- **Vim-like keybindings** throughout

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
ytui                        # start with empty search
ytui -search "go tutorial"  # search immediately on startup
```

## Keybindings

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
| `p` | Play video (opens quality picker) |
| `d` | Download video |
| `c` | View comments |
| `l` / `h` | Expand / collapse replies |
| `L` | Load more (comments / search results) |
| `a` | Authenticate (extract browser cookies) |
| `o` | Open video in browser |
| `y` | Copy video URL to clipboard |
| `r` | Refresh current view |
| `?` | Toggle help |
| `q` | Quit |
| `Ctrl+c` | Force quit |

## Configuration

Config file: `~/.config/ytui/config.toml` (respects `$XDG_CONFIG_HOME`)

All settings are optional -- defaults are used for any missing values.

```toml
[player]
command = "mpv"                    # player command (default: "mpv")
args = ["--no-terminal"]           # extra arguments passed to the player

[download]
command = "yt-dlp"                 # download command (default: "yt-dlp")
output_dir = "~/Videos/ytui"       # download directory (default: "~/Videos/ytui")
format = ""                        # default yt-dlp format string

[auth]
browser = "brave"                  # browser: brave, chrome, chromium, firefox, edge
auth_on_startup = false            # auto-authenticate on launch (default: false)
```

## Authentication

ytui extracts YouTube session cookies from your browser to access subscriptions and feed. Cookies are held in memory only -- never written to disk.

Supported browsers: **Brave**, **Chrome**, **Chromium**, **Firefox**, **Edge**. Configure which one in the `[auth]` section of the config file.

- Press `a` to authenticate manually
- Set `auth_on_startup = true` in config for automatic authentication

## License

MIT
