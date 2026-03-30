package youtube

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	innertubego "github.com/nezbut/innertube-go"
	"github.com/tidwall/gjson"
)

// MusicClient provides YouTube Music API access via InnerTube WEB_REMIX client.
type MusicClient struct {
	it *innertubego.InnerTube
}

// NewMusicClient creates a new YouTube Music client.
// Pass a custom httpClient with a cookie jar for authenticated requests.
func NewMusicClient(httpClient *http.Client) (*MusicClient, error) {
	it, err := innertubego.NewInnerTube(httpClient, "WEB_REMIX", "1.20230724.00.00", "", "", "", nil, true)
	if err != nil {
		return nil, fmt.Errorf("music innertube init: %w", err)
	}
	return &MusicClient{it: it}, nil
}

// Search searches YouTube Music for the given query.
func (c *MusicClient) Search(ctx context.Context, query string) (*MusicSearchResult, error) {
	raw, err := c.it.Search(ctx, &query, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("music search: %w", err)
	}
	return parseMusicSearchResponse(raw)
}

// Browse fetches a YouTube Music browse page (home, album, artist, playlist).
func (c *MusicClient) Browse(ctx context.Context, browseID string) (map[string]interface{}, error) {
	raw, err := c.it.Browse(ctx, &browseID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("music browse: %w", err)
	}
	return raw, nil
}

// GetAlbumTracks fetches the tracks for an album browseID.
// Returns the first track's videoID and playlistID for playback.
func (c *MusicClient) GetAlbumTracks(ctx context.Context, browseID string) ([]MusicItem, string, error) {
	raw, err := c.Browse(ctx, browseID)
	if err != nil {
		return nil, "", err
	}
	data, err := toGJSON(raw)
	if err != nil {
		return nil, "", err
	}

	var tracks []MusicItem
	var playlistID string

	sections := data.Get("contents.twoColumnBrowseResultsRenderer.secondaryContents.sectionListRenderer.contents")
	sections.ForEach(func(_, section gjson.Result) bool {
		shelf := section.Get("musicShelfRenderer")
		if !shelf.Exists() {
			return true
		}
		shelf.Get("contents").ForEach(func(_, entry gjson.Result) bool {
			mrlir := entry.Get("musicResponsiveListItemRenderer")
			if !mrlir.Exists() {
				return true
			}
			title := mrlir.Get("flexColumns.0.musicResponsiveListItemFlexColumnRenderer.text.runs.0.text").String()
			vid := mrlir.Get("overlay.musicItemThumbnailOverlayRenderer.content.musicPlayButtonRenderer.playNavigationEndpoint.watchEndpoint.videoId").String()
			plid := mrlir.Get("overlay.musicItemThumbnailOverlayRenderer.content.musicPlayButtonRenderer.playNavigationEndpoint.watchEndpoint.playlistId").String()

			if playlistID == "" && plid != "" {
				playlistID = plid
			}

			tracks = append(tracks, MusicItem{
				Type:    MusicSong,
				Title:   title,
				VideoID: vid,
			})
			return true
		})
		return false // only first shelf
	})

	return tracks, playlistID, nil
}

func parseMusicSearchResponse(raw map[string]interface{}) (*MusicSearchResult, error) {
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}

	result := &MusicSearchResult{}

	sections := data.Get("contents.tabbedSearchResultsRenderer.tabs.0.tabRenderer.content.sectionListRenderer.contents")
	sections.ForEach(func(_, section gjson.Result) bool {
		// Top result card (artist/album)
		card := section.Get("musicCardShelfRenderer")
		if card.Exists() {
			item := parseMusicCard(card)
			result.TopResult = &item
			return true
		}

		// Shelf of results
		shelf := section.Get("musicShelfRenderer")
		if shelf.Exists() {
			title := shelf.Get("title.runs.0.text").String()
			var items []MusicItem
			shelf.Get("contents").ForEach(func(_, entry gjson.Result) bool {
				mrlir := entry.Get("musicResponsiveListItemRenderer")
				if mrlir.Exists() {
					items = append(items, parseMusicListItem(mrlir))
				}
				return true
			})
			result.Shelves = append(result.Shelves, MusicShelf{
				Title: title,
				Items: items,
			})
		}
		return true
	})

	return result, nil
}

func parseMusicCard(card gjson.Result) MusicItem {
	title := card.Get("title.runs.0.text").String()
	browseID := card.Get("title.runs.0.navigationEndpoint.browseEndpoint.browseId").String()

	var subtitleParts []string
	card.Get("subtitle.runs").ForEach(func(_, r gjson.Result) bool {
		subtitleParts = append(subtitleParts, r.Get("text").String())
		return true
	})
	subtitle := strings.Join(subtitleParts, "")

	itemType := detectMusicItemType(subtitle, browseID)

	item := MusicItem{
		Type:     itemType,
		Title:    title,
		Subtitle: subtitle,
		BrowseID: browseID,
	}

	card.Get("thumbnail.musicThumbnailRenderer.thumbnail.thumbnails").ForEach(func(_, t gjson.Result) bool {
		item.Thumbnails = append(item.Thumbnails, Thumbnail{
			URL:    t.Get("url").String(),
			Width:  int(t.Get("width").Int()),
			Height: int(t.Get("height").Int()),
		})
		return true
	})

	return item
}

func parseMusicListItem(mrlir gjson.Result) MusicItem {
	cols := mrlir.Get("flexColumns")

	// Column 0: title
	title := cols.Get("0.musicResponsiveListItemFlexColumnRenderer.text.runs.0.text").String()

	// Column 1: subtitle with type info (e.g., "Song • Artist" or "Album • Artist • Year")
	var subtitleParts []string
	cols.Get("1.musicResponsiveListItemFlexColumnRenderer.text.runs").ForEach(func(_, r gjson.Result) bool {
		subtitleParts = append(subtitleParts, r.Get("text").String())
		return true
	})
	subtitle := strings.Join(subtitleParts, "")

	// Detect type from first word of subtitle
	itemType := MusicSong
	if len(subtitleParts) > 0 {
		first := strings.TrimSpace(subtitleParts[0])
		switch first {
		case "Album":
			itemType = MusicAlbum
		case "Artist":
			itemType = MusicArtist
		case "Playlist":
			itemType = MusicPlaylist
		case "Video":
			itemType = MusicVideo
		case "Song":
			itemType = MusicSong
		}
	}

	// Get videoId (for playable items)
	videoID := mrlir.Get("overlay.musicItemThumbnailOverlayRenderer.content.musicPlayButtonRenderer.playNavigationEndpoint.watchEndpoint.videoId").String()

	// Get browseId (for browsable items)
	browseID := mrlir.Get("navigationEndpoint.browseEndpoint.browseId").String()

	item := MusicItem{
		Type:     itemType,
		Title:    title,
		Subtitle: subtitle,
		VideoID:  videoID,
		BrowseID: browseID,
	}

	mrlir.Get("thumbnail.musicThumbnailRenderer.thumbnail.thumbnails").ForEach(func(_, t gjson.Result) bool {
		item.Thumbnails = append(item.Thumbnails, Thumbnail{
			URL:    t.Get("url").String(),
			Width:  int(t.Get("width").Int()),
			Height: int(t.Get("height").Int()),
		})
		return true
	})

	return item
}

func detectMusicItemType(subtitle, browseID string) MusicItemType {
	lower := strings.ToLower(subtitle)
	if strings.Contains(lower, "artist") {
		return MusicArtist
	}
	if strings.HasPrefix(browseID, "MPRE") {
		return MusicAlbum
	}
	if strings.HasPrefix(browseID, "VL") {
		return MusicPlaylist
	}
	return MusicSong
}
